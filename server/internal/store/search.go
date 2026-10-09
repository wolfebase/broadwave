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
	airings, err := s.searchAirings(ctx, match, titleMatch(query), from, limit)
	if err != nil {
		return nil, nil, err
	}
	recordings, err := s.searchRecordings(ctx, match, limit)
	if err != nil {
		return nil, nil, err
	}
	return airings, recordings, nil
}

// searchAirings reads listings on the channels the guide shows: one row per
// channel when two tuners carry it, and none that are hidden. An encrypted 3.0
// station is searched too, since it records as its clear twin, unless the twin
// lists the same show. Shows named by the words come before ones that only
// mention them.
func (s *Store) searchAirings(ctx context.Context, match, title string, from time.Time, limit int) ([]AiringHit, error) {
	all, err := s.Channels(ctx, false)
	if err != nil {
		return nil, err
	}
	shown := map[int64]bool{}
	for _, ch := range all {
		if ch.Present && ch.Enabled && !ch.Hidden && ch.SameAs == 0 {
			shown[ch.ID] = true
		}
	}
	standIn := map[int64]int64{}
	args := []any{match, from.UTC().Format(time.RFC3339)}
	var marks []string
	for _, ch := range all {
		if !shown[ch.ID] && (ch.PlaysAs == 0 || !shown[ch.PlaysAs]) {
			continue
		}
		if !shown[ch.ID] {
			standIn[ch.ID] = ch.PlaysAs
		}
		marks = append(marks, "?")
		args = append(args, ch.ID)
	}
	if len(marks) == 0 {
		return []AiringHit{}, nil
	}
	// A stand-in's duplicates are dropped below, so read a few more.
	args = append(args, title, limit+len(standIn)*4)
	rows, err := s.db.QueryContext(ctx, `
SELECT a.id, a.channel_id, a.title, a.subtitle, a.description, a.category, a.starts_at, a.ends_at,
	a.program_id, a.is_new, a.image_url, a.image_width, a.image_height, a.season, a.episode, a.episode_label, a.original_air, a.series_id,
	a.is_live, a.is_premiere, a.is_finale, a.rating, a.cast_list, a.game_id, c.guide_number, c.guide_name
FROM airing_search
JOIN airings a ON a.id = airing_search.rowid
JOIN channels c ON c.id = a.channel_id
WHERE airing_search MATCH ? AND a.ends_at > ? AND a.channel_id IN (`+strings.Join(marks, ",")+`)
ORDER BY a.id NOT IN (SELECT rowid FROM airing_search WHERE airing_search MATCH ?), a.starts_at, a.id
LIMIT ?`, args...)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return withoutTwinRepeats(out, standIn, limit), nil
}

// withoutTwinRepeats drops an encrypted station's listing when its clear twin
// lists the same show at the same time.
func withoutTwinRepeats(hits []AiringHit, standIn map[int64]int64, limit int) []AiringHit {
	type key struct {
		channel int64
		title   string
		start   int64
	}
	listed := map[key]bool{}
	for _, hit := range hits {
		if _, ok := standIn[hit.ChannelID]; !ok {
			listed[key{hit.ChannelID, hit.Title, hit.Start.Unix()}] = true
		}
	}
	out := []AiringHit{}
	for _, hit := range hits {
		if twin, ok := standIn[hit.ChannelID]; ok && listed[key{twin, hit.Title, hit.Start.Unix()}] {
			continue
		}
		if len(out) == limit {
			break
		}
		out = append(out, hit)
	}
	return out
}

func (s *Store) searchRecordings(ctx context.Context, match string, limit int) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT `+recordingColumns+`
FROM recording_search
JOIN recordings r ON r.id = recording_search.rowid
LEFT JOIN progress p ON p.recording_id = r.id
WHERE recording_search MATCH ?
ORDER BY r.started_at DESC
LIMIT ?`, match, limit)
	if err != nil {
		return nil, err
	}
	out, err := scanRecordings(rows)
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []Recording{}
	}
	return out, nil
}

// ftsMatch turns typed words into a prefix query. Punctuation that FTS treats
// as syntax is dropped so a title with a quote cannot break the search.
// titleMatch is ftsMatch on the title column alone.
func titleMatch(query string) string {
	words := strings.Fields(ftsMatch(query))
	for i, w := range words {
		words[i] = "title : " + w
	}
	return strings.Join(words, " ")
}

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
