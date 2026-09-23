package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type Marker struct {
	ID          int64   `json:"id"`
	RecordingID int64   `json:"recordingId"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
}

type VirtualChannel struct {
	ID         int64   `json:"id"`
	Number     string  `json:"number"`
	Name       string  `json:"name"`
	OrderMode  string  `json:"orderMode,omitempty"`
	RuleTitle  string  `json:"ruleTitle,omitempty"`
	Recordings []int64 `json:"recordings"`
}

func (s *Store) migrateLibrary() {
	_, _ = s.db.Exec(`
CREATE TABLE IF NOT EXISTS markers (
	id INTEGER PRIMARY KEY,
	recording_id INTEGER NOT NULL,
	start_sec REAL NOT NULL,
	end_sec REAL NOT NULL
);
CREATE TABLE IF NOT EXISTS virtual_channels (
	id INTEGER PRIMARY KEY,
	number TEXT NOT NULL,
	name TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS virtual_items (
	id INTEGER PRIMARY KEY,
	virtual_id INTEGER NOT NULL,
	recording_id INTEGER NOT NULL,
	position INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS progress (
	recording_id INTEGER PRIMARY KEY,
	position_sec REAL NOT NULL,
	updated_at TEXT NOT NULL
);
`)
	_, _ = s.db.Exec(`ALTER TABLE virtual_channels ADD COLUMN order_mode TEXT NOT NULL DEFAULT 'custom'`)
	_, _ = s.db.Exec(`ALTER TABLE virtual_channels ADD COLUMN rule_title TEXT NOT NULL DEFAULT ''`)
}

func (s *Store) SaveProgress(ctx context.Context, recordingID int64, seconds float64) error {
	if seconds < 0 {
		seconds = 0
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO progress (recording_id, position_sec, updated_at) VALUES (?, ?, ?)
ON CONFLICT(recording_id) DO UPDATE SET position_sec = excluded.position_sec, updated_at = excluded.updated_at`,
		recordingID, seconds, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) Progress(ctx context.Context, recordingID int64) (float64, error) {
	var seconds float64
	err := s.db.QueryRowContext(ctx, `SELECT position_sec FROM progress WHERE recording_id = ?`, recordingID).Scan(&seconds)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return seconds, err
}

func (s *Store) AddMarker(ctx context.Context, recordingID int64, start, end float64) (Marker, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO markers (recording_id, start_sec, end_sec) VALUES (?, ?, ?)`, recordingID, start, end)
	if err != nil {
		return Marker{}, err
	}
	id, _ := res.LastInsertId()
	return Marker{ID: id, RecordingID: recordingID, Start: start, End: end}, nil
}

func (s *Store) Markers(ctx context.Context, recordingID int64) ([]Marker, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, recording_id, start_sec, end_sec FROM markers WHERE recording_id = ? ORDER BY start_sec`, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Marker
	for rows.Next() {
		var m Marker
		if err := rows.Scan(&m.ID, &m.RecordingID, &m.Start, &m.End); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) DeleteMarker(ctx context.Context, id int64) (int64, error) {
	var recordingID int64
	if err := s.db.QueryRowContext(ctx, `SELECT recording_id FROM markers WHERE id = ?`, id).Scan(&recordingID); err != nil {
		return 0, err
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM markers WHERE id = ?`, id)
	return recordingID, err
}

func (s *Store) ReplaceMarkers(ctx context.Context, recordingID int64, rows []Marker) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `DELETE FROM markers WHERE recording_id = ?`, recordingID); err != nil {
		return err
	}
	for _, row := range rows {
		if row.End <= row.Start {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO markers (recording_id, start_sec, end_sec) VALUES (?, ?, ?)`, recordingID, row.Start, row.End); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) DeletePass(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM passes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateVirtual(ctx context.Context, number, name string, recordingIDs []int64) (VirtualChannel, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO virtual_channels (number, name) VALUES (?, ?)`, number, name)
	if err != nil {
		return VirtualChannel{}, err
	}
	id, _ := res.LastInsertId()
	for i, rec := range recordingIDs {
		if _, err := s.db.ExecContext(ctx, `INSERT INTO virtual_items (virtual_id, recording_id, position) VALUES (?, ?, ?)`, id, rec, i); err != nil {
			return VirtualChannel{}, err
		}
	}
	return s.virtualByID(ctx, id)
}

func (s *Store) Virtuals(ctx context.Context) ([]VirtualChannel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, number, name, order_mode, rule_title FROM virtual_channels ORDER BY number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []VirtualChannel
	for rows.Next() {
		var v VirtualChannel
		if err := rows.Scan(&v.ID, &v.Number, &v.Name, &v.OrderMode, &v.RuleTitle); err != nil {
			return nil, err
		}
		items, err := s.db.QueryContext(ctx, `SELECT recording_id FROM virtual_items WHERE virtual_id = ? ORDER BY position`, v.ID)
		if err != nil {
			return nil, err
		}
		for items.Next() {
			var id int64
			if err := items.Scan(&id); err != nil {
				items.Close()
				return nil, err
			}
			v.Recordings = append(v.Recordings, id)
		}
		items.Close()
		if v.Recordings == nil {
			v.Recordings = []int64{}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Virtual(ctx context.Context, id int64) (VirtualChannel, error) {
	return s.virtualByID(ctx, id)
}

func (s *Store) virtualByID(ctx context.Context, id int64) (VirtualChannel, error) {
	all, err := s.Virtuals(ctx)
	if err != nil {
		return VirtualChannel{}, err
	}
	for _, v := range all {
		if v.ID == id {
			return v, nil
		}
	}
	return VirtualChannel{}, sql.ErrNoRows
}

func (s *Store) UpdateVirtual(ctx context.Context, id int64, orderMode, ruleTitle string, recordingIDs []int64) (VirtualChannel, error) {
	if orderMode == "" {
		orderMode = "custom"
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE virtual_channels SET order_mode = ?, rule_title = ? WHERE id = ?`, orderMode, ruleTitle, id); err != nil {
		return VirtualChannel{}, err
	}
	if recordingIDs != nil {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM virtual_items WHERE virtual_id = ?`, id); err != nil {
			return VirtualChannel{}, err
		}
		for i, rec := range recordingIDs {
			if _, err := s.db.ExecContext(ctx, `INSERT INTO virtual_items (virtual_id, recording_id, position) VALUES (?, ?, ?)`, id, rec, i); err != nil {
				return VirtualChannel{}, err
			}
		}
	}
	return s.virtualByID(ctx, id)
}

func (s *Store) BackupTo(ctx context.Context, path string) error {
	escaped := strings.ReplaceAll(path, "'", "''")
	_, err := s.db.ExecContext(ctx, fmt.Sprintf(`VACUUM INTO '%s'`, escaped))
	return err
}

func (s *Store) RestoreFrom(ctx context.Context, path string) error {
	escaped := strings.ReplaceAll(path, "'", "''")
	if _, err := s.db.ExecContext(ctx, fmt.Sprintf(`ATTACH DATABASE '%s' AS incoming`, escaped)); err != nil {
		return err
	}
	defer func() { _, _ = s.db.ExecContext(ctx, `DETACH DATABASE incoming`) }()
	tables := []string{"settings", "passes", "markers", "virtual_channels", "virtual_items", "progress", "seen_programs", "sources", "skipped_airings"}
	for _, table := range tables {
		if _, err := s.db.ExecContext(ctx, `DELETE FROM `+table); err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, `INSERT INTO `+table+` SELECT * FROM incoming.`+table); err != nil {
			return fmt.Errorf("%s: %w", table, err)
		}
	}
	return nil
}
