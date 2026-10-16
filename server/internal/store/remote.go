package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	// SettingRemoteURL is the https address that reaches this server from
	// outside the house, such as a Cloudflare tunnel's public hostname.
	SettingRemoteURL = "remoteUrl"
	// SettingTunnelToken is the Cloudflare tunnel token, as PUT /settings
	// takes it. It is kept in the config folder's secrets file, never in the
	// catalog; a catalog from before that may still hold one. See TakeTunnelToken.
	SettingTunnelToken = "tunnelToken"

	inviteTTL        = 7 * 24 * time.Hour
	invitePendingMax = 20
	inviteLen        = 12
	// inviteAlphabet leaves out 0, 1, I, and O, which read alike on a TV.
	inviteAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"
)

// Invite is a long code for someone outside the house. It lasts a week and
// pairs one device.
type Invite struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Scopes    []string  `json:"scopes"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NormalizeRemoteURL turns "tv.example.com" or "https://tv.example.com/" into
// "https://tv.example.com". An empty value clears the setting.
func NormalizeRemoteURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	bad := errors.New("the public address is a name such as tv.example.com")
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.Trim(u.Path, "/") != "" {
		return "", bad
	}
	if u.Scheme != "https" {
		return "", errors.New("the public address needs https")
	}
	host := strings.ToLower(strings.TrimRight(u.Hostname(), "."))
	if host == "" || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return "", bad
	}
	for _, c := range host {
		if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '.') {
			return "", bad
		}
	}
	out := "https://" + host
	if p := u.Port(); p != "" && p != "443" {
		out += ":" + p
	}
	return out, nil
}

// RemoteHost is the name in a normalized remote URL, or "".
func RemoteHost(remoteURL string) string {
	u, err := url.Parse(remoteURL)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

// TakeTunnelToken removes a tunnel token the catalog still holds and returns
// it: one saved before tokens moved out of the catalog, or one a restored
// backup brought back.
func (s *Store) TakeTunnelToken(ctx context.Context) (string, error) {
	var token string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, SettingTunnelToken).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM settings WHERE key = ?`, SettingTunnelToken); err != nil {
		return "", err
	}
	return strings.TrimSpace(token), nil
}

// CleanTunnelToken accepts the token or the whole command Cloudflare shows
// ("cloudflared service install eyJ…"), and keeps only the token. An empty
// value clears it.
func CleanTunnelToken(raw string) (string, error) {
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return "", nil
	}
	token := ""
	for _, f := range fields {
		if strings.HasPrefix(f, "eyJ") {
			token = f
		}
	}
	if token == "" && len(fields) == 1 {
		token = fields[0]
	}
	bad := errors.New("that is not a Cloudflare tunnel token. Copy the token from the tunnel's install command")
	if token == "" || len(token) > 4096 {
		return "", bad
	}
	body, err := base64.StdEncoding.DecodeString(padBase64(token))
	if err != nil {
		body, err = base64.RawURLEncoding.DecodeString(strings.TrimRight(token, "="))
	}
	if err != nil {
		return "", bad
	}
	var parsed struct {
		Account string `json:"a"`
		Tunnel  string `json:"t"`
		Secret  string `json:"s"`
	}
	if json.Unmarshal(body, &parsed) != nil || parsed.Tunnel == "" || parsed.Secret == "" {
		return "", bad
	}
	return token, nil
}

func padBase64(s string) string {
	s = strings.TrimRight(s, "=")
	if n := len(s) % 4; n != 0 {
		s += strings.Repeat("=", 4-n)
	}
	return s
}

// CreateInvite stores a week-long code that pairs one device with scopes.
// The code is returned once and only its hash is kept.
func (s *Store) CreateInvite(ctx context.Context, name string, scopes []string, now time.Time) (Invite, string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Invite{}, "", errors.New("say who the invite is for")
	}
	if len([]rune(name)) > 63 {
		return Invite{}, "", errors.New("name is too long")
	}
	scopes, err := cleanScopes(scopes)
	if err != nil {
		return Invite{}, "", err
	}
	if err := s.expirePairings(ctx, now); err != nil {
		return Invite{}, "", err
	}
	var pending int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pairings WHERE mode = 'invite' AND state = 'pending'`).Scan(&pending); err != nil {
		return Invite{}, "", err
	}
	if pending >= invitePendingMax {
		return Invite{}, "", errors.New("Too many invites are waiting. Remove one first.")
	}
	id, err := newID()
	if err != nil {
		return Invite{}, "", err
	}
	var last error
	for range 5 {
		code, err := inviteCode()
		if err != nil {
			return Invite{}, "", err
		}
		expires := now.UTC().Add(inviteTTL)
		_, err = s.db.ExecContext(ctx, `INSERT INTO pairings (
id, mode, code_hash, name, kind, scopes, created_at, expires_at, state
) VALUES (?, 'invite', ?, ?, 'other', ?, ?, ?, 'pending')`,
			id, s.hashCode(code), name, joinScopes(scopes),
			now.UTC().Format(time.RFC3339), expires.Format(time.RFC3339))
		if err == nil {
			created, _ := time.Parse(time.RFC3339, now.UTC().Format(time.RFC3339))
			exp, _ := time.Parse(time.RFC3339, expires.Format(time.RFC3339))
			return Invite{ID: id, Name: name, Scopes: scopes, CreatedAt: created, ExpiresAt: exp}, FormatInvite(code), nil
		}
		if !pairingUnique(err) {
			return Invite{}, "", err
		}
		last = err
	}
	return Invite{}, "", last
}

// Invites lists the invites nobody has used yet.
func (s *Store) Invites(ctx context.Context, now time.Time) ([]Invite, error) {
	if err := s.expirePairings(ctx, now); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, scopes, created_at, expires_at FROM pairings
WHERE mode = 'invite' AND state = 'pending' ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Invite{}
	for rows.Next() {
		var inv Invite
		var scopes, created, expires string
		if err := rows.Scan(&inv.ID, &inv.Name, &scopes, &created, &expires); err != nil {
			return nil, err
		}
		inv.Scopes = splitScopes(scopes)
		inv.CreatedAt, _ = time.Parse(time.RFC3339, created)
		inv.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
		out = append(out, inv)
	}
	return out, rows.Err()
}

// DeleteInvite stops an invite nobody has used.
func (s *Store) DeleteInvite(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE pairings SET state = 'expired' WHERE id = ? AND mode = 'invite' AND state = 'pending'`, id)
	return err
}

// ClaimInvite pairs the device that typed an invite code.
func (s *Store) ClaimInvite(ctx context.Context, code, name, kind string, now time.Time) (Pairing, ClientDevice, string, error) {
	return s.finishCode(ctx, "invite", code, name, kind, nil, now)
}

// IsInviteCode reports whether raw has the shape of an invite rather than a
// six-digit pairing code.
func IsInviteCode(raw string) bool {
	_, ok := normalizeInvite(raw)
	return ok
}

// FormatInvite groups a code in fours so it can be read aloud.
func FormatInvite(code string) string {
	var parts []string
	for i := 0; i < len(code); i += 4 {
		end := min(i+4, len(code))
		parts = append(parts, code[i:end])
	}
	return strings.Join(parts, "-")
}

func normalizeInvite(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range strings.ToUpper(raw) {
		if r == ' ' || r == '-' {
			continue
		}
		if !strings.ContainsRune(inviteAlphabet, r) {
			return "", false
		}
		b.WriteRune(r)
	}
	if b.Len() != inviteLen {
		return "", false
	}
	return b.String(), true
}

func inviteCode() (string, error) {
	// 32 letters divide 256 evenly, so a byte's low five bits are uniform.
	buf := make([]byte, inviteLen)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, inviteLen)
	for i, b := range buf {
		out[i] = inviteAlphabet[int(b)%len(inviteAlphabet)]
	}
	return string(out), nil
}
