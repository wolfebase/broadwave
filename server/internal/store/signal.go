package store

import (
	"context"
	"sort"
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
// The order matches the guide, so a shown 3.0 channel sits beside its 1.0 twin
// instead of with the 100s. Text order on guide_number put 104.1 after 100.1.
func (s *Store) ChannelSignals(ctx context.Context) ([]ChannelSignal, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT c.id, c.guide_number, c.guide_name, c.frequency_hz,
	sig.strength, sig.quality, sig.symbol, sig.locked, sig.checked_at
FROM channels c
LEFT JOIN channel_signals sig ON sig.channel_id = c.id
WHERE c.present = 1`)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	channels, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	rank := make(map[int64]int, len(channels))
	for i, ch := range channels {
		rank[ch.ID] = i
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, okI := rank[out[i].ChannelID]
		rj, okJ := rank[out[j].ChannelID]
		if okI != okJ {
			return okI
		}
		if !okI {
			return out[i].ChannelID < out[j].ChannelID
		}
		return ri < rj
	})
	return out, nil
}

// SaveChannelSignal writes one channel's reading, for a stream the device
// tuned on its own (ATSC 3.0), whose frequency is not one row's alone.
func (s *Store) SaveChannelSignal(ctx context.Context, channelID int64, strength, quality, symbol int, when time.Time) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO channel_signals (channel_id, strength, quality, symbol, locked, checked_at) VALUES (?, ?, ?, ?, 1, ?)
ON CONFLICT(channel_id) DO UPDATE SET
	strength = excluded.strength,
	quality = excluded.quality,
	symbol = excluded.symbol,
	locked = 1,
	checked_at = excluded.checked_at`,
		channelID, strength, quality, symbol, when.UTC().Format(time.RFC3339))
	return err
}

// SetChannelStreamInfo stores what a tune read of a channel's stream: a
// caption service, and the main sound's layout. An empty layout keeps the
// one stored.
func (s *Store) SetChannelStreamInfo(ctx context.Context, channelID int64, captions bool, audio string) error {
	c := 0
	if captions {
		c = 1
	}
	_, err := s.db.ExecContext(ctx, `
UPDATE channels SET captions = ?, audio_layout = CASE WHEN ? != '' THEN ? ELSE audio_layout END WHERE id = ?`,
		c, audio, audio, channelID)
	return err
}
