package store

import (
	"context"
	"encoding/binary"
	"time"
)

// Spot is a commercial's picture prints, one per second.
type Spot struct {
	ID          int64
	RecordingID int64
	Prints      []uint64
}

// maxSpots keeps the library to about a megabyte, the spots seen most lately.
const maxSpots = 5000

func (s *Store) Spots(ctx context.Context) ([]Spot, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, recording_id, prints FROM spots ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Spot
	for rows.Next() {
		var sp Spot
		var blob []byte
		if err := rows.Scan(&sp.ID, &sp.RecordingID, &blob); err != nil {
			return nil, err
		}
		for i := 0; i+8 <= len(blob); i += 8 {
			sp.Prints = append(sp.Prints, binary.LittleEndian.Uint64(blob[i:]))
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// AddSpots stores a recording's new spots and drops the ones seen longest ago
// past maxSpots.
func (s *Store) AddSpots(ctx context.Context, recordingID int64, spots [][]uint64) error {
	if len(spots) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, prints := range spots {
		blob := make([]byte, 8*len(prints))
		for i, p := range prints {
			binary.LittleEndian.PutUint64(blob[8*i:], p)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO spots (prints, recording_id, seen_at) VALUES (?, ?, ?)`, blob, recordingID, now); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM spots WHERE id NOT IN (SELECT id FROM spots ORDER BY seen_at DESC, id DESC LIMIT ?)`, maxSpots); err != nil {
		return err
	}
	return tx.Commit()
}

// SeeSpots records that spots played again.
func (s *Store) SeeSpots(ctx context.Context, ids []int64) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		if _, err := s.db.ExecContext(ctx, `UPDATE spots SET hits = hits + 1, seen_at = ? WHERE id = ?`, now, id); err != nil {
			return err
		}
	}
	return nil
}
