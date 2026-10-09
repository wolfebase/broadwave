package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	// SettingDeviceAuth is "1" when a device token is required.
	// Missing and "0" leave the LAN API open.
	SettingDeviceAuth = "deviceAuth"

	ScopeWatch  = "watch"
	ScopeRecord = "record"
	ScopeAdmin  = "admin"

	pairTTL        = 10 * time.Minute
	pairPendingMax = 20
	pairDenyAfter  = 5
)

var (
	ErrPairCode    = errors.New("That code does not match.")
	ErrPairExpired = errors.New("That code expired.")
	ErrPairDenied  = errors.New("That code was stopped after too many tries.")
	ErrPairFull    = errors.New("Too many devices are waiting to pair.")
)

// ClientDevice is a phone, TV, or browser paired with this server.
// The bearer token is not on the struct.
type ClientDevice struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Kind       string    `json:"kind"`
	Scopes     []string  `json:"scopes"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt,omitempty"`
	RevokedAt  time.Time `json:"revokedAt,omitempty"`
}

// Pairing is a code waiting to be approved or claimed.
type Pairing struct {
	ID        string
	Mode      string
	Name      string
	Kind      string
	Scopes    []string
	ExpiresAt time.Time
	State     string
	DeviceID  string
}

// ScopeAllows reports whether the granted scopes cover need.
// admin covers record and watch. record covers watch.
func ScopeAllows(have []string, need string) bool {
	needRank := scopeRank(need)
	if needRank == 0 {
		return false
	}
	best := 0
	for _, scope := range have {
		if rank := scopeRank(scope); rank > best {
			best = rank
		}
	}
	return best >= needRank
}

func scopeRank(scope string) int {
	switch scope {
	case ScopeWatch:
		return 1
	case ScopeRecord:
		return 2
	case ScopeAdmin:
		return 3
	default:
		return 0
	}
}

// InsertClient saves a device and returns its raw token once.
func (s *Store) InsertClient(ctx context.Context, name, kind string, scopes []string, now time.Time) (ClientDevice, string, error) {
	name, err := cleanClientName(name)
	if err != nil {
		return ClientDevice{}, "", err
	}
	scopes, err = cleanScopes(scopes)
	if err != nil {
		return ClientDevice{}, "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return ClientDevice{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	dev, token, err := insertClientTx(ctx, tx, name, kind, scopes, now)
	if err != nil {
		return ClientDevice{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return ClientDevice{}, "", err
	}
	return dev, token, nil
}

// ClientByToken finds the device for a raw bearer token.
// revoked is true when the hash matches a revoked row. An unknown token
// returns an empty device and revoked false.
func (s *Store) ClientByToken(ctx context.Context, token string) (ClientDevice, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return ClientDevice{}, false, nil
	}
	dev, revoked, err := s.scanClient(ctx, `SELECT id, name, kind, scopes, created_at, last_seen_at, revoked_at
FROM client_devices WHERE token_hash = ?`, hashSecret(token))
	if errors.Is(err, sql.ErrNoRows) {
		return ClientDevice{}, false, nil
	}
	return dev, revoked, err
}

// Clients lists devices that have not been revoked, oldest first.
func (s *Store) Clients(ctx context.Context) ([]ClientDevice, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, kind, scopes, created_at, last_seen_at, revoked_at
FROM client_devices WHERE revoked_at = '' ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ClientDevice
	for rows.Next() {
		dev, err := scanClientRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, dev)
	}
	return out, rows.Err()
}

// HasAdmin reports whether an unrevoked admin device exists.
func (s *Store) HasAdmin(ctx context.Context) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM client_devices
WHERE revoked_at = '' AND (',' || scopes || ',') LIKE '%,admin,%'`).Scan(&n)
	return n > 0, err
}

// TouchClient records that a device called, when the previous stamp is stale.
func (s *Store) TouchClient(ctx context.Context, id string, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE client_devices SET last_seen_at = ? WHERE id = ? AND revoked_at = ''`,
		now.UTC().Format(time.RFC3339), id)
	return err
}

// RevokeClient stops a device's token. A second revoke is ErrNoRows.
func (s *Store) RevokeClient(ctx context.Context, id string, now time.Time) error {
	res, err := s.db.ExecContext(ctx, `UPDATE client_devices SET revoked_at = ? WHERE id = ? AND revoked_at = ''`,
		now.UTC().Format(time.RFC3339), id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// StartPairing opens a waiting code. mode is "show" (the device displays it
// and polls with pollSecret) or "claim" (an admin displays it).
// The code and pollSecret are returned once and are not stored.
func (s *Store) StartPairing(ctx context.Context, mode, name, kind string, scopes []string, now time.Time) (id, code, pollSecret string, expires time.Time, err error) {
	if mode != "show" && mode != "claim" {
		return "", "", "", time.Time{}, errors.New("pairing mode must be show or claim")
	}
	if mode == "show" {
		name, err = cleanClientName(name)
		if err != nil {
			return "", "", "", time.Time{}, err
		}
	} else {
		name = strings.TrimSpace(name)
		if len([]rune(name)) > 63 {
			return "", "", "", time.Time{}, errors.New("name is too long")
		}
	}
	scopes, err = cleanScopes(scopes)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	kind = cleanKind(kind)
	if err := s.expirePairings(ctx, now); err != nil {
		return "", "", "", time.Time{}, err
	}
	var pending int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pairings WHERE state = 'pending' AND expires_at > ?`,
		now.UTC().Format(time.RFC3339)).Scan(&pending); err != nil {
		return "", "", "", time.Time{}, err
	}
	if pending >= pairPendingMax {
		return "", "", "", time.Time{}, ErrPairFull
	}
	id, err = newID()
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	code, err = uniqueCode(ctx, s, now)
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	var secretHash string
	if mode == "show" {
		pollSecret, err = newSecret("ps_")
		if err != nil {
			return "", "", "", time.Time{}, err
		}
		secretHash = hashSecret(pollSecret)
	}
	expires = now.UTC().Add(pairTTL)
	_, err = s.db.ExecContext(ctx, `INSERT INTO pairings (
id, mode, code_hash, poll_secret_hash, name, kind, scopes, created_at, expires_at, state
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending')`,
		id, mode, hashSecret(code), secretHash, name, kind, joinScopes(scopes),
		now.UTC().Format(time.RFC3339), expires.Format(time.RFC3339))
	if err != nil {
		return "", "", "", time.Time{}, err
	}
	return id, code, pollSecret, expires, nil
}

// ApproveShow matches a code a device is showing, saves the device, and
// returns the raw token once. The caller keeps the token in memory until
// the device polls. An empty scopes list is watch. A list replaces whatever
// the device asked for. The device's own request is not a grant.
func (s *Store) ApproveShow(ctx context.Context, code string, scopes []string, now time.Time) (Pairing, ClientDevice, string, error) {
	return s.finishCode(ctx, "show", code, "", "", scopes, now)
}

// ClaimCode matches a code an admin created. The device supplies its name and kind.
func (s *Store) ClaimCode(ctx context.Context, code, name, kind string, now time.Time) (Pairing, ClientDevice, string, error) {
	return s.finishCode(ctx, "claim", code, name, kind, nil, now)
}

// PollPairing checks the poll secret for a show-mode pairing.
// A wrong secret counts toward denying that pairing.
func (s *Store) PollPairing(ctx context.Context, id, secret string, now time.Time) (Pairing, error) {
	if err := s.expirePairings(ctx, now); err != nil {
		return Pairing{}, err
	}
	p, err := s.pairingByID(ctx, id)
	if err != nil {
		return Pairing{}, err
	}
	if p.Mode != "show" {
		return Pairing{}, ErrPairCode
	}
	var secretHash string
	if err := s.db.QueryRowContext(ctx, `SELECT poll_secret_hash FROM pairings WHERE id = ?`, id).Scan(&secretHash); err != nil {
		return Pairing{}, err
	}
	if !hashEqual(secretHash, hashSecret(secret)) {
		if p.State == "pending" {
			if err := s.notePairFailure(ctx, id); err != nil {
				return Pairing{}, err
			}
		}
		return Pairing{}, ErrPairCode
	}
	if p.State == "denied" {
		return p, ErrPairDenied
	}
	return p, nil
}

func (s *Store) finishCode(ctx context.Context, mode, code, name, kind string, scopes []string, now time.Time) (Pairing, ClientDevice, string, error) {
	norm, ok := normalizeCode(code)
	if !ok {
		return Pairing{}, ClientDevice{}, "", ErrPairCode
	}
	if err := s.expirePairings(ctx, now); err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, code_hash, name, kind, scopes, expires_at, state, attempts
FROM pairings WHERE mode = ? AND state IN ('pending', 'denied', 'expired')`, mode)
	if err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	defer rows.Close()
	want := hashSecret(norm)
	var (
		found        bool
		id           string
		storedName   string
		storedKind   string
		storedScopes string
		expires      string
		state        string
		attempts     int
	)
	for rows.Next() {
		var hash, exp, st string
		var att int
		var rowID, rowName, rowKind, rowScopes string
		if err := rows.Scan(&rowID, &hash, &rowName, &rowKind, &rowScopes, &exp, &st, &att); err != nil {
			return Pairing{}, ClientDevice{}, "", err
		}
		// A pending code wins over an older expired one with the same digits.
		if hashEqual(hash, want) && (!found || state != "pending") {
			found = true
			id, storedName, storedKind, storedScopes = rowID, rowName, rowKind, rowScopes
			expires, state, attempts = exp, st, att
		}
	}
	if err := rows.Err(); err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	if !found {
		return Pairing{}, ClientDevice{}, "", ErrPairCode
	}
	if state == "denied" || attempts >= pairDenyAfter {
		return Pairing{}, ClientDevice{}, "", ErrPairDenied
	}
	expAt, _ := time.Parse(time.RFC3339, expires)
	if state == "expired" || !expAt.After(now) {
		return Pairing{}, ClientDevice{}, "", ErrPairExpired
	}
	if mode == "claim" {
		var nameErr error
		storedName, nameErr = cleanClientName(name)
		if nameErr != nil {
			return Pairing{}, ClientDevice{}, "", nameErr
		}
		if kind != "" {
			storedKind = cleanKind(kind)
		}
		// Claim keeps the scopes the admin stored with the code.
		scopes = splitScopes(storedScopes)
	} else {
		// Show: omitted scopes are watch, not the scopes the new device requested.
		cleaned, scopeErr := cleanScopes(scopes)
		if scopeErr != nil {
			return Pairing{}, ClientDevice{}, "", scopeErr
		}
		scopes = cleaned
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	defer func() { _ = tx.Rollback() }()
	// The row may have been consumed between the read and the write.
	res, err := tx.ExecContext(ctx, `UPDATE pairings SET state = 'approved' WHERE id = ? AND state = 'pending'`, id)
	if err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Pairing{}, ClientDevice{}, "", ErrPairCode
	}
	dev, token, err := insertClientTx(ctx, tx, storedName, storedKind, scopes, now)
	if err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE pairings SET device_id = ?, scopes = ? WHERE id = ?`, dev.ID, joinScopes(scopes), id); err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	if err := tx.Commit(); err != nil {
		return Pairing{}, ClientDevice{}, "", err
	}
	return Pairing{ID: id, Mode: mode, Name: dev.Name, Kind: dev.Kind, Scopes: dev.Scopes, ExpiresAt: expAt, State: "approved", DeviceID: dev.ID}, dev, token, nil
}

func (s *Store) notePairFailure(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE pairings SET attempts = attempts + 1,
state = CASE WHEN attempts + 1 >= ? THEN 'denied' ELSE state END
WHERE id = ? AND state = 'pending'`, pairDenyAfter, id)
	return err
}

func (s *Store) expirePairings(ctx context.Context, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE pairings SET state = 'expired' WHERE state = 'pending' AND expires_at <= ?`,
		now.UTC().Format(time.RFC3339))
	return err
}

func (s *Store) pairingByID(ctx context.Context, id string) (Pairing, error) {
	var p Pairing
	var scopes, expires string
	err := s.db.QueryRowContext(ctx, `SELECT id, mode, name, kind, scopes, expires_at, state, device_id
FROM pairings WHERE id = ?`, id).Scan(&p.ID, &p.Mode, &p.Name, &p.Kind, &scopes, &expires, &p.State, &p.DeviceID)
	if errors.Is(err, sql.ErrNoRows) {
		return Pairing{}, ErrPairCode
	}
	if err != nil {
		return Pairing{}, err
	}
	p.Scopes = splitScopes(scopes)
	p.ExpiresAt, _ = time.Parse(time.RFC3339, expires)
	return p, nil
}

func (s *Store) scanClient(ctx context.Context, query string, args ...any) (ClientDevice, bool, error) {
	var dev ClientDevice
	var scopes, created, seen, revoked string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&dev.ID, &dev.Name, &dev.Kind, &scopes, &created, &seen, &revoked)
	if err != nil {
		return ClientDevice{}, false, err
	}
	dev.Scopes = splitScopes(scopes)
	dev.CreatedAt, _ = time.Parse(time.RFC3339, created)
	dev.LastSeenAt = parseStamp(seen)
	dev.RevokedAt = parseStamp(revoked)
	return dev, revoked != "", nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanClientRow(rows scanner) (ClientDevice, error) {
	var dev ClientDevice
	var scopes, created, seen, revoked string
	if err := rows.Scan(&dev.ID, &dev.Name, &dev.Kind, &scopes, &created, &seen, &revoked); err != nil {
		return ClientDevice{}, err
	}
	dev.Scopes = splitScopes(scopes)
	dev.CreatedAt, _ = time.Parse(time.RFC3339, created)
	dev.LastSeenAt = parseStamp(seen)
	dev.RevokedAt = parseStamp(revoked)
	return dev, nil
}

func insertClientTx(ctx context.Context, tx *sql.Tx, name, kind string, scopes []string, now time.Time) (ClientDevice, string, error) {
	id, err := newID()
	if err != nil {
		return ClientDevice{}, "", err
	}
	token, err := newSecret("bw_")
	if err != nil {
		return ClientDevice{}, "", err
	}
	stamp := now.UTC().Format(time.RFC3339)
	_, err = tx.ExecContext(ctx, `INSERT INTO client_devices (id, name, kind, scopes, token_hash, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, name, cleanKind(kind), joinScopes(scopes), hashSecret(token), stamp)
	if err != nil {
		return ClientDevice{}, "", err
	}
	created, _ := time.Parse(time.RFC3339, stamp)
	return ClientDevice{ID: id, Name: name, Kind: cleanKind(kind), Scopes: append([]string(nil), scopes...), CreatedAt: created}, token, nil
}

func uniqueCode(ctx context.Context, s *Store, now time.Time) (string, error) {
	for range 5 {
		code, err := sixDigits()
		if err != nil {
			return "", err
		}
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pairings WHERE code_hash = ? AND state = 'pending' AND expires_at > ?`,
			hashSecret(code), now.UTC().Format(time.RFC3339)).Scan(&n); err != nil {
			return "", err
		}
		if n == 0 {
			return code, nil
		}
	}
	return "", ErrPairFull
}

func hashSecret(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func hashEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func newID() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func newSecret(prefix string) (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func sixDigits() (string, error) {
	// 2^32 is not divisible by 1_000_000. Reject the remainder so each code
	// is equally likely.
	const span = 1000000
	const limit = (1 << 32) / span * span
	for {
		var buf [4]byte
		if _, err := rand.Read(buf[:]); err != nil {
			return "", err
		}
		n := binary.BigEndian.Uint32(buf[:])
		if n < limit {
			return fmt.Sprintf("%06d", n%span), nil
		}
	}
}

func normalizeCode(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range raw {
		if r == ' ' || r == '-' {
			continue
		}
		if r < '0' || r > '9' {
			return "", false
		}
		b.WriteRune(r)
	}
	if b.Len() != 6 {
		return "", false
	}
	return b.String(), true
}

func cleanClientName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("a name is required")
	}
	if len([]rune(name)) > 63 {
		return "", errors.New("name is too long")
	}
	for _, r := range name {
		if r < 0x20 {
			return "", errors.New("name has a character this server cannot show")
		}
	}
	return name, nil
}

func cleanKind(kind string) string {
	switch strings.TrimSpace(kind) {
	case "phone", "tv", "web", "other":
		return strings.TrimSpace(kind)
	default:
		return "other"
	}
}

func cleanScopes(in []string) ([]string, error) {
	if len(in) == 0 {
		return []string{ScopeWatch}, nil
	}
	have := map[string]bool{}
	for _, scope := range in {
		scope = strings.TrimSpace(strings.ToLower(scope))
		if scope == "" {
			continue
		}
		switch scope {
		case ScopeWatch, ScopeRecord, ScopeAdmin:
			have[scope] = true
		default:
			return nil, fmt.Errorf("unknown scope %q", scope)
		}
	}
	if len(have) == 0 {
		return []string{ScopeWatch}, nil
	}
	var out []string
	for _, scope := range []string{ScopeWatch, ScopeRecord, ScopeAdmin} {
		if have[scope] {
			out = append(out, scope)
		}
	}
	return out, nil
}

func joinScopes(scopes []string) string { return strings.Join(scopes, ",") }

func splitScopes(raw string) []string {
	if raw == "" {
		return nil
	}
	return strings.Split(raw, ",")
}

func parseStamp(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}
