package store

import (
	"context"
	"time"
)

const (
	settingLastGuidePull   = "lastGuidePull"
	settingNextGuidePull   = "nextGuidePull"
	settingLastManualGuide = "lastManualGuidePull"
)

// GuideSchedule is when listings were last pulled and when the next automatic
// pull is due. Times are zero when the catalog has never recorded them.
func (s *Store) GuideSchedule(ctx context.Context) (last, next, lastManual time.Time, err error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return time.Time{}, time.Time{}, time.Time{}, err
	}
	return parseSettingTime(settings[settingLastGuidePull]), parseSettingTime(settings[settingNextGuidePull]), parseSettingTime(settings[settingLastManualGuide]), nil
}

// SetGuideSchedule records a successful pull and the next automatic one.
func (s *Store) SetGuideSchedule(ctx context.Context, last, next time.Time) error {
	return s.writeSettings(ctx, map[string]string{
		settingLastGuidePull: last.UTC().Format(time.RFC3339),
		settingNextGuidePull: next.UTC().Format(time.RFC3339),
	})
}

// SetNextGuidePull moves only the automatic deadline, used after a failed pull.
func (s *Store) SetNextGuidePull(ctx context.Context, next time.Time) error {
	return s.writeSettings(ctx, map[string]string{
		settingNextGuidePull: next.UTC().Format(time.RFC3339),
	})
}

// SetManualGuidePull records a manual refresh so the next one can be rate-limited.
func (s *Store) SetManualGuidePull(ctx context.Context, at time.Time) error {
	return s.writeSettings(ctx, map[string]string{
		settingLastManualGuide: at.UTC().Format(time.RFC3339),
	})
}

func parseSettingTime(raw string) time.Time {
	if raw == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}
	}
	return t
}
