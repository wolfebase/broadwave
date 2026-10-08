package store

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
)

// EpisodePrints are the sound prints of a recording's first and last minutes,
// and where its listing starts and ends in the file (ShowEnd 0: not known).
type EpisodePrints struct {
	Head, Tail []uint32
	TailFrom   float64
	ShowStart  float64
	ShowEnd    float64
}

func packPrints(prints []uint32) []byte {
	blob := make([]byte, 4*len(prints))
	for i, p := range prints {
		binary.LittleEndian.PutUint32(blob[4*i:], p)
	}
	return blob
}

func unpackPrints(blob []byte) []uint32 {
	out := make([]uint32, 0, len(blob)/4)
	for i := 0; i+4 <= len(blob); i += 4 {
		out = append(out, binary.LittleEndian.Uint32(blob[i:]))
	}
	return out
}

// SaveEpisodePrints stores a recording's prints; empty ones say it was
// listened to and had no sound. A recording deleted meanwhile gets none: its
// id goes to the next recording.
func (s *Store) SaveEpisodePrints(ctx context.Context, recordingID int64, p EpisodePrints) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO episode_prints (recording_id, head, tail, tail_from, show_start, show_end)
SELECT ?, ?, ?, ?, ?, ? WHERE EXISTS (SELECT 1 FROM recordings WHERE id = ?)
ON CONFLICT (recording_id) DO UPDATE SET head = excluded.head, tail = excluded.tail, tail_from = excluded.tail_from,
	show_start = excluded.show_start, show_end = excluded.show_end`,
		recordingID, packPrints(p.Head), packPrints(p.Tail), p.TailFrom, p.ShowStart, p.ShowEnd, recordingID)
	return err
}

// EpisodePrints returns a recording's prints, or false when it has none.
func (s *Store) EpisodePrints(ctx context.Context, recordingID int64) (EpisodePrints, bool, error) {
	var head, tail []byte
	var p EpisodePrints
	err := s.db.QueryRowContext(ctx, `SELECT head, tail, tail_from, show_start, show_end FROM episode_prints WHERE recording_id = ?`, recordingID).
		Scan(&head, &tail, &p.TailFrom, &p.ShowStart, &p.ShowEnd)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	p.Head, p.Tail = unpackPrints(head), unpackPrints(tail)
	return p, true, nil
}

// SetEpisodeEnds stores where a recording's intro and end titles are.
func (s *Store) SetEpisodeEnds(ctx context.Context, recordingID int64, introStart, introEnd, credits float64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET intro_start = ?, intro_end = ?, credits_start = ? WHERE id = ?`,
		introStart, introEnd, credits, recordingID)
	return err
}
