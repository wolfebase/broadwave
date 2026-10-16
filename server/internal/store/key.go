package store

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const keyFile = "broadwave.key"

// loadKey reads the server secret, or makes one the first time.
func loadKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, keyFile)
	raw, err := os.ReadFile(path)
	if err == nil {
		key, decErr := hex.DecodeString(strings.TrimSpace(string(raw)))
		if decErr == nil && len(key) >= 32 {
			return key, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, []byte(hex.EncodeToString(key)+"\n"), 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

func (s *Store) mac(label string, parts ...string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(label))
	for _, p := range parts {
		m.Write([]byte{0})
		m.Write([]byte(p))
	}
	return m.Sum(nil)
}

// hashCode stores a pairing or invite code. Keyed, so a copy of the catalog
// alone cannot be searched for the code.
func (s *Store) hashCode(code string) string {
	if len(s.key) == 0 {
		return hashSecret(code)
	}
	return hex.EncodeToString(s.mac("code", code))
}

// MediaLinkTTL is how long a media link plays.
const MediaLinkTTL = 24 * time.Hour

// SignMedia makes the mk value that lets a receiver without the device token
// read paths under prefix until exp, for as long as device is not revoked.
func (s *Store) SignMedia(prefix, device string, exp time.Time) string {
	var head [8]byte
	binary.BigEndian.PutUint64(head[:], uint64(exp.Unix()))
	payload := base64.RawURLEncoding.EncodeToString(append(head[:], []byte(device)...))
	sig := base64.RawURLEncoding.EncodeToString(s.mac("media", prefix, payload)[:18])
	return payload + "." + sig
}

// MediaDevice checks an mk value for prefix and returns the device it was
// made for. A link for a revoked device, an expired one, or another prefix
// is refused.
func (s *Store) MediaDevice(ctx context.Context, mk, prefix string, now time.Time) (ClientDevice, bool, error) {
	payload, sig, ok := strings.Cut(mk, ".")
	if !ok || len(s.key) == 0 {
		return ClientDevice{}, false, nil
	}
	want := base64.RawURLEncoding.EncodeToString(s.mac("media", prefix, payload)[:18])
	if !hmac.Equal([]byte(sig), []byte(want)) {
		return ClientDevice{}, false, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(raw) <= 8 {
		return ClientDevice{}, false, nil
	}
	if now.Unix() >= int64(binary.BigEndian.Uint64(raw[:8])) {
		return ClientDevice{}, false, nil
	}
	dev, revoked, err := s.scanClient(ctx, `SELECT id, name, kind, scopes, created_at, last_seen_at, revoked_at
FROM client_devices WHERE id = ?`, string(raw[8:]))
	if errors.Is(err, sql.ErrNoRows) || revoked {
		return ClientDevice{}, false, nil
	}
	if err != nil {
		return ClientDevice{}, false, err
	}
	return dev, true, nil
}
