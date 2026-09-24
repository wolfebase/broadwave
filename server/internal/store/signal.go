package store

import (
	"context"
	"time"
)

// ChannelSignal is the last antenna reading for one channel.
type ChannelSignal struct {
	ChannelID   int64
	GuideNumber string
	GuideName   string
	FrequencyHz int
	Strength    int
	Quality     int
	Symbol      int
	Locked      bool
	CheckedAt   time.Time
	HasReading  bool
}

// SaveFrequencySignal writes one lock to every channel on that frequency.
func (s *Store) SaveFrequencySignal(ctx context.Context, freq int, lock bool, strength, quality, symbol int, when time.Time) error {
	locked := 0
	if lock {
		locked = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_signals (channel_id, strength, quality, symbol, locked, checked_at)
SELECT id, ?, ?, ?, ?, ? FROM channels WHERE frequency_hz = ? AND present = 1
ON CONFLICT(channel_id) DO UPDATE SET
	strength = excluded.strength,
	quality = excluded.quality,
	symbol = excluded.symbol,
	locked = excluded.locked,
	checked_at = excluded.checked_at`,
		strength, quality, symbol, locked, when.UTC().Format(time.RFC3339), freq)
	return err
}

// ChannelSignals lists present channels and any reading stored for them.
func (s *Store) ChannelSignals(ctx context.Context) ([]ChannelSignal, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id, c.guide_number, c.guide_name, c.frequency_hz,
	sig.strength, sig.quality, sig.symbol, sig.locked, sig.checked_at
FROM channels c
LEFT JOIN channel_signals sig ON sig.channel_id = c.id
WHERE c.present = 1
ORDER BY c.guide_number`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChannelSignal
	for rows.Next() {
		var row ChannelSignal
		var strength, quality, symbol, locked *int
		var checked *string
		if err := rows.Scan(&row.ChannelID, &row.GuideNumber, &row.GuideName, &row.FrequencyHz, &strength, &quality, &symbol, &locked, &checked); err != nil {
			return nil, err
		}
		if strength != nil && checked != nil {
			row.Strength, row.Quality, row.Symbol = *strength, *quality, *symbol
			row.Locked = locked != nil && *locked != 0
			row.CheckedAt, _ = time.Parse(time.RFC3339, *checked)
			row.HasReading = true
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
