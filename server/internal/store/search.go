package store

import (
	"context"
	"strings"
	"time"
)

// AiringHit is a guide listing that matched a search, with the channel it is on.
type AiringHit struct {
	Airing
	GuideNumber string `json:"guideNumber"`
	ChannelName string `json:"channelName"`
}

// Search finds upcoming listings and recordings. A blank query matches nothing.
func (s *Store) Search(ctx context.Context, query string, from time.Time, limit int) ([]AiringHit, []Recording, error) {
	match := ftsMatch(query)
	if match == "" {
		return []AiringHit{}, []Recording{}, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 40
	}
	airings, err := s.searchAirings(ctx, match, from, limit)
	if err != nil {
		return nil, nil, err
	}
	recordings, err := s.searchRecordings(ctx, match, limit)
	if err != nil {
		return nil, nil, err
	}
	return airings, recordings, nil
}

func (s *Store) searchAirings(ctx context.Context, match string, from time.Time, limit int) ([]AiringHit, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.channel_id, a.title, a.subtitle, a.description, a.category, a.starts_at, a.ends_at,
	a.program_id, a.is_new, a.image_url, a.image_width, a.image_height, a.season, a.episode, a.episode_label, a.original_air, a.series_id,
	a.is_live, a.is_premiere, a.is_finale, a.rating, a.cast_list, a.game_id, c.guide_number, c.guide_name
FROM airing_search
JOIN airings a ON a.id = airing_search.rowid
JOIN channels c ON c.id = a.channel_id
WHERE airing_search MATCH ? AND a.ends_at > ?
ORDER BY a.starts_at
LIMIT ?`, match, from.UTC().Format(time.RFC3339), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AiringHit
	for rows.Next() {
		var hit AiringHit
		var start, end string
		var isNew, isLive, isPremiere, isFinale int
		if err := rows.Scan(&hit.ID, &hit.ChannelID, &hit.Title, &hit.Subtitle, &hit.Description, &hit.Category, &start, &end,
			&hit.ProgramID, &isNew, &hit.ImageURL, &hit.ImageWidth, &hit.ImageHeight, &hit.Season, &hit.Episode, &hit.EpisodeLabel, &hit.OriginalAir, &hit.SeriesID,
			&isLive, &isPremiere, &isFinale, &hit.Rating, &hit.Cast, &hit.GameID, &hit.GuideNumber, &hit.ChannelName); err != nil {
			return nil, err
		}
		hit.New = isNew != 0
		hit.Live = isLive != 0
		hit.Premiere = isPremiere != 0
		hit.Finale = isFinale != 0
		hit.Start, _ = time.Parse(time.RFC3339, start)
		hit.End, _ = time.Parse(time.RFC3339, end)
		out = append(out, hit)
	}
	if out == nil {
		out = []AiringHit{}
	}
	return out, rows.Err()
}

func (s *Store) searchRecordings(ctx context.Context, match string, limit int) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT r.id, r.channel_id, r.guide_number, r.title, r.status, r.error, r.started_at, r.ends_at, r.ended_at, r.duration_sec,
	r.subtitle, r.description, r.category, r.program_id, r.watched
FROM recording_search
JOIN recordings r ON r.id = recording_search.rowid
WHERE recording_search MATCH ?
ORDER BY r.started_at DESC
LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Recording
	for rows.Next() {
		var rec Recording
		var started, ends, ended string
		if err := rows.Scan(&rec.ID, &rec.ChannelID, &rec.GuideNumber, &rec.Title, &rec.Status, &rec.Error, &started, &ends, &ended, &rec.Duration,
			&rec.Subtitle, &rec.Description, &rec.Category, &rec.ProgramID, &rec.Watched); err != nil {
			return nil, err
		}
		rec.StartedAt, _ = time.Parse(time.RFC3339, started)
		if ends != "" {
			t, _ := time.Parse(time.RFC3339, ends)
			rec.EndsAt = &t
		}
		if ended != "" {
			t, _ := time.Parse(time.RFC3339, ended)
			rec.EndedAt = &t
		}
		out = append(out, rec)
	}
	if out == nil {
		out = []Recording{}
	}
	return out, rows.Err()
}

// ftsMatch turns typed words into a prefix query. Punctuation that FTS treats
// as syntax is dropped so a title with a quote cannot break the search.
func ftsMatch(query string) string {
	fields := strings.Fields(query)
	var parts []string
	for _, field := range fields {
		var b strings.Builder
		for _, r := range field {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' {
				b.WriteRune(r)
			}
		}
		word := b.String()
		if len(word) < 2 {
			continue
		}
		parts = append(parts, `"`+word+`"*`)
	}
	return strings.Join(parts, " ")
}
