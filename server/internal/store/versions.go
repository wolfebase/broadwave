package store

import (
	"context"
	"time"
)

// Versions counts changes to the data the apps cache. Each number only
// grows; triggers in the catalog bump it on every write that changes what a
// client would see (migration 0040).
type Versions struct {
	// Guide is the listings.
	Guide int64 `json:"guide"`
	// Channels is the channel list and the tuners behind it.
	Channels int64 `json:"channels"`
	// Recordings is the recordings library.
	Recordings int64 `json:"recordings"`
	// Schedule is passes, skips, and episodes already recorded or deleted.
	Schedule int64 `json:"schedule"`
	// Settings is the server settings.
	Settings int64 `json:"settings"`
}

// DataVersions reads every counter in one query.
func (s *Store) DataVersions(ctx context.Context) (Versions, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, version FROM data_versions`)
	if err != nil {
		return Versions{}, err
	}
	defer rows.Close()
	var v Versions
	for rows.Next() {
		var name string
		var n int64
		if err := rows.Scan(&name, &n); err != nil {
			return Versions{}, err
		}
		switch name {
		case "guide":
			v.Guide = n
		case "channels":
			v.Channels = n
		case "recordings":
			v.Recordings = n
		case "schedule":
			v.Schedule = n
		case "settings":
			v.Settings = n
		}
	}
	return v, rows.Err()
}

// airingsKeep is how much of the past guide is kept. The guide shows earlier
// today, and a recording links its listing for a day or so.
const airingsKeep = 3 * 24 * time.Hour

// PruneAirings removes listings that ended long ago. A catalog that never
// pruned kept every listing it was ever sent, and each guide window paid to
// skip them.
func (s *Store) PruneAirings(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM airings WHERE ends_at < ?`, now.Add(-airingsKeep).UTC().Format(time.RFC3339))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
