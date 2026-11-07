package store

import (
	"context"
	"time"
)

// RecordingWait is a scheduled airing that could not start. The retry keeps
// going, and when the airing ends without a recording the wait becomes a
// failed recording that says why.
type RecordingWait struct {
	ChannelID   int64
	Start       time.Time
	End         time.Time
	Until       time.Time // the end of the airing plus its padding
	PassID      int64
	Title       string
	Subtitle    string
	Description string
	Category    string
	ProgramID   string
	GameID      string
	// Reason is what the plan line says while it waits.
	Reason string
	// Failure is what the failed recording says if it never starts.
	Failure string
	Since   time.Time
}

// PutRecordingWait notes a wait, or the latest reason for one already noted.
func (s *Store) PutRecordingWait(ctx context.Context, w RecordingWait) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO recording_waits (channel_id, airing_start, airing_end, until_at, pass_id, title, subtitle, description, category, program_id, game_id, reason, failure, since)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(channel_id, airing_start) DO UPDATE SET reason = excluded.reason, failure = excluded.failure`,
		w.ChannelID, w.Start.Unix(), w.End.Unix(), w.Until.Unix(), w.PassID, w.Title, w.Subtitle, w.Description, w.Category, w.ProgramID, w.GameID, w.Reason, w.Failure, w.Since.Unix())
	return err
}

// RecordingWaits lists every open wait.
func (s *Store) RecordingWaits(ctx context.Context) ([]RecordingWait, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT channel_id, airing_start, airing_end, until_at, pass_id, title, subtitle, description, category, program_id, game_id, reason, failure, since
FROM recording_waits ORDER BY airing_start`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordingWait
	for rows.Next() {
		var w RecordingWait
		var start, end, until, since int64
		if err := rows.Scan(&w.ChannelID, &start, &end, &until, &w.PassID, &w.Title, &w.Subtitle, &w.Description, &w.Category, &w.ProgramID, &w.GameID, &w.Reason, &w.Failure, &since); err != nil {
			return nil, err
		}
		w.Start, w.End, w.Until, w.Since = time.Unix(start, 0), time.Unix(end, 0), time.Unix(until, 0), time.Unix(since, 0)
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeleteRecordingWait closes one wait.
func (s *Store) DeleteRecordingWait(ctx context.Context, channelID int64, start time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM recording_waits WHERE channel_id = ? AND airing_start = ?`, channelID, start.Unix())
	return err
}
