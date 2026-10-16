package cloudflare

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Secret is the one tunnel this server runs.
type Secret struct {
	// Token is the tunnel's run token. It runs this tunnel and nothing else.
	Token string `json:"token"`
	// Source is "broadwave" for a tunnel made by Sign in with Cloudflare,
	// or "pasted" for a token the owner copied from the dashboard.
	Source string `json:"source"`
	// Routes is where a Broadwave tunnel's route lives: "cloudflare" or
	// "local". A local tunnel runs with --url.
	Routes    string    `json:"routes,omitempty"`
	TunnelID  string    `json:"tunnelId,omitempty"`
	AccountID string    `json:"accountId,omitempty"`
	ZoneID    string    `json:"zoneId,omitempty"`
	Hostname  string    `json:"hostname,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
}

// Made reports a tunnel Broadwave made, as opposed to a pasted token.
func (s Secret) Made() bool { return s.Token != "" && s.Source == SourceBroadwave }

const (
	SourceBroadwave = "broadwave"
	SourcePasted    = "pasted"
)

// Vault keeps the Secret in <config>/secrets/cloudflare.json. The catalog,
// and so every catalog backup, never holds it. A Vault with no Path keeps it
// in memory, for tests.
type Vault struct {
	Path string
	mu   sync.Mutex
	mem  Secret
}

// NewVault is the vault under the config folder.
func NewVault(configDir string) *Vault {
	return &Vault{Path: filepath.Join(configDir, "secrets", "cloudflare.json")}
}

// Load returns the saved secret, or the zero Secret when there is none.
func (v *Vault) Load() (Secret, error) {
	if v == nil {
		return Secret{}, nil
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.Path == "" {
		return v.mem, nil
	}
	raw, err := os.ReadFile(v.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Secret{}, nil
	}
	if err != nil {
		return Secret{}, err
	}
	var s Secret
	if err := json.Unmarshal(raw, &s); err != nil {
		return Secret{}, err
	}
	s.Token = strings.TrimSpace(s.Token)
	return s, nil
}

// Save replaces the secret. The folder is 0700 and the file 0600.
func (v *Vault) Save(s Secret) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.Path == "" {
		v.mem = s
		return nil
	}
	dir := filepath.Dir(v.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	// MkdirAll leaves an existing folder's mode alone.
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cloudflare-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(append(raw, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), v.Path)
}

// Clear removes the secret.
func (v *Vault) Clear() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.Path == "" {
		v.mem = Secret{}
		return nil
	}
	err := os.Remove(v.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
