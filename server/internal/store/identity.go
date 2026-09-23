package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// Identity names this server to clients. The id survives restarts and renames,
// so a paired app recognizes the same server on a new address.
type Identity struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Store) Identity(ctx context.Context, defaultName string) (Identity, error) {
	var id Identity
	err := s.db.QueryRowContext(ctx, `SELECT server_id, name FROM server_identity WHERE id = 1`).Scan(&id.ID, &id.Name)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Identity{}, err
	}
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return Identity{}, err
	}
	id = Identity{ID: hex.EncodeToString(buf), Name: strings.TrimSpace(defaultName)}
	if id.Name == "" {
		id.Name = "Waveguide"
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO server_identity (id, server_id, name, created_at) VALUES (1, ?, ?, ?)
ON CONFLICT(id) DO NOTHING`, id.ID, id.Name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return Identity{}, err
	}
	return s.Identity(ctx, defaultName)
}

func (s *Store) RenameServer(ctx context.Context, name string) error {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 63 {
		return errors.New("server name must be 1 to 63 characters")
	}
	res, err := s.db.ExecContext(ctx, `UPDATE server_identity SET name = ? WHERE id = 1`, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}
