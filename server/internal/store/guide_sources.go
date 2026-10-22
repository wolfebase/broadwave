package store

import (
	"context"
	"encoding/json"
	"time"
)

// SettingGuideSources holds each listing source's own schedule. It is the
// server's bookkeeping, not a setting anyone edits.
const SettingGuideSources = "guideSources"

// GuideSource is one listing source's pull schedule and its last outcome.
type GuideSource struct {
	// Next is when the source is pulled again.
	Next time.Time `json:"next"`
	// OK is the last pull that brought listings.
	OK time.Time `json:"ok,omitzero"`
	// Fails counts pulls that failed since the last good one; the wait
	// before the next one grows with it.
	Fails   int       `json:"fails,omitempty"`
	Error   string    `json:"error,omitempty"`
	ErrorAt time.Time `json:"errorAt,omitzero"`
	// Config is a fingerprint of the account or address the last pull used.
	// A different one is a settings change, and the source is pulled now.
	Config string `json:"config,omitempty"`
	// Rows is how many listings the last good pull brought.
	Rows int `json:"rows,omitempty"`
}

// GuideSources reads every source's schedule. A source never pulled is missing.
func (s *Store) GuideSources(ctx context.Context) (map[string]GuideSource, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return nil, err
	}
	out := map[string]GuideSource{}
	if raw := settings[SettingGuideSources]; raw != "" {
		_ = json.Unmarshal([]byte(raw), &out)
	}
	return out, nil
}

// SetGuideSources replaces every source's schedule.
func (s *Store) SetGuideSources(ctx context.Context, sources map[string]GuideSource) error {
	raw, err := json.Marshal(sources)
	if err != nil {
		return err
	}
	return s.writeSettings(ctx, map[string]string{SettingGuideSources: string(raw)})
}

// FeedRows reads the listings one guide source wrote that have not ended.
// A source that is not pulled this time keeps these.
func (s *Store) FeedRows(ctx context.Context, source string, now time.Time) ([]Airing, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+airingColumns+` FROM airings a WHERE a.guide_source = ? AND a.ends_at > ? ORDER BY a.channel_id, a.starts_at`,
		source, now.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	return scanAiringRows(rows)
}
