package store

import (
	"context"
	"time"
)

// ChannelGuide is one channel's listings in a window: how many, when the
// last one ends, and the guide that wrote the earliest listing naming a source.
type ChannelGuide struct {
	ChannelID int64
	Airings   int
	Until     time.Time
	Source    string
}

const channelsListedSQL = `
SELECT DISTINCT channel_id FROM airings
WHERE ends_at > ? AND starts_at < ?`

const guideCountSQL = `
SELECT channel_id, COUNT(*), MAX(ends_at)
FROM airings
WHERE ends_at > ? AND starts_at < ?
GROUP BY channel_id`

// The source is the earliest listing that names one, in starts_at order.
// MAX(guide_source) would pick a different guide.
const guideSourceSQL = `
SELECT channel_id, guide_source FROM (
	SELECT channel_id, guide_source,
		ROW_NUMBER() OVER (PARTITION BY channel_id ORDER BY starts_at, id) AS n
	FROM airings
	WHERE ends_at > ? AND starts_at < ? AND guide_source != ''
) WHERE n = 1`

// A listing on now counts once. One that starts within the next minute counts
// again, because it is also still ahead. The scan treats two as enough.
const listingDepthSQL = `
SELECT channel_id, SUM((starts_at < ? AND ends_at > ?) + (starts_at > ?))
FROM airings
WHERE ends_at > ? AND starts_at < ?
GROUP BY channel_id`

// ChannelsListedBetween is every channel with a listing that overlaps [from, to).
func (s *Store) ChannelsListedBetween(ctx context.Context, from, to time.Time) (map[int64]bool, error) {
	rows, err := s.db.QueryContext(ctx, channelsListedSQL, from.UTC().Format(time.RFC3339), to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// GuideWindow summarizes listings that overlap [from, to). Total counts every
// listing, including a channel that has left the lineup.
func (s *Store) GuideWindow(ctx context.Context, from, to time.Time) (total int, until time.Time, rows []ChannelGuide, err error) {
	fromArg := from.UTC().Format(time.RFC3339)
	toArg := to.UTC().Format(time.RFC3339)
	var out []ChannelGuide
	at := map[int64]int{}
	counted, err := s.db.QueryContext(ctx, guideCountSQL, fromArg, toArg)
	if err != nil {
		return 0, time.Time{}, nil, err
	}
	for counted.Next() {
		var row ChannelGuide
		var end string
		if err := counted.Scan(&row.ChannelID, &row.Airings, &end); err != nil {
			counted.Close()
			return 0, time.Time{}, nil, err
		}
		row.Until, _ = time.Parse(time.RFC3339, end)
		if row.Until.After(until) {
			until = row.Until
		}
		total += row.Airings
		at[row.ChannelID] = len(out)
		out = append(out, row)
	}
	countErr := counted.Err()
	counted.Close()
	if countErr != nil {
		return 0, time.Time{}, nil, countErr
	}
	sources, err := s.db.QueryContext(ctx, guideSourceSQL, fromArg, toArg)
	if err != nil {
		return 0, time.Time{}, nil, err
	}
	defer sources.Close()
	for sources.Next() {
		var id int64
		var source string
		if err := sources.Scan(&id, &source); err != nil {
			return 0, time.Time{}, nil, err
		}
		if i, ok := at[id]; ok {
			out[i].Source = source
		}
	}
	return total, until, out, sources.Err()
}

// ListingDepth is how many listings a channel has on now or still ahead,
// inside the next six hours. A show that starts within a minute counts twice.
func (s *Store) ListingDepth(ctx context.Context, now time.Time) (map[int64]int, error) {
	rows, err := s.db.QueryContext(ctx, listingDepthSQL,
		now.Add(time.Minute).UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
		now.UTC().Format(time.RFC3339),
		now.Add(-time.Minute).UTC().Format(time.RFC3339),
		now.Add(6*time.Hour).UTC().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]int{}
	for rows.Next() {
		var id int64
		var n int
		if err := rows.Scan(&id, &n); err != nil {
			return nil, err
		}
		out[id] = n
	}
	return out, rows.Err()
}
