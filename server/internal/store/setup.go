package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// setupWindow is how long a brand-new catalog stays eligible for the wizard.
// Older catalogs are treated as already set up so an upgrade never interrupts them.
const setupWindow = 24 * time.Hour

// NeedsSetup is true only for a catalog that has never finished setup, is
// younger than a day, and has no passes or recordings. Favorites imported
// from the tuner lineup do not count: discovery sets those before the wizard
// can finish.
func NeedsSetup(setupComplete string, createdAt, now time.Time, passes, recordings int) bool {
	if setupComplete == "1" {
		return false
	}
	if passes > 0 || recordings > 0 {
		return false
	}
	if createdAt.IsZero() || now.Sub(createdAt) < setupWindow {
		return true
	}
	return false
}

// CreatedAt is when this catalog was first opened. A missing row is a zero time.
func (s *Store) CreatedAt(ctx context.Context) (time.Time, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT created_at FROM server_identity WHERE id = 1`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, nil
	}
	return t, nil
}

// HouseholdMarks counts passes and recordings. Those are created in Waveguide,
// unlike channel favorites, which the tuner lineup can set on first discovery.
func (s *Store) HouseholdMarks(ctx context.Context) (passes, recordings int, err error) {
	err = s.db.QueryRowContext(ctx, `
SELECT
  (SELECT COUNT(*) FROM passes),
  (SELECT COUNT(*) FROM recordings)`).Scan(&passes, &recordings)
	return
}

// ApplySetupDefault stamps setupComplete=1 on catalogs that should never see
// the wizard, and reports whether a fresh catalog still needs it.
func (s *Store) ApplySetupDefault(ctx context.Context, now time.Time) (bool, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return false, err
	}
	created, err := s.CreatedAt(ctx)
	if err != nil {
		return false, err
	}
	passes, recordings, err := s.HouseholdMarks(ctx)
	if err != nil {
		return false, err
	}
	needs := NeedsSetup(settings["setupComplete"], created, now, passes, recordings)
	if !needs && settings["setupComplete"] != "1" {
		if err := s.PutSettings(ctx, map[string]string{"setupComplete": "1"}); err != nil {
			return false, err
		}
	}
	return needs, nil
}
