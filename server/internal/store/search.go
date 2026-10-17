package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
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
	if limit <= 0 || limit > 100 {
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
	var ids []int64
	for _, ch := range all {
		if !shown[ch.ID] && (ch.PlaysAs == 0 || !shown[ch.PlaysAs]) {
			continue
		}
		if !shown[ch.ID] {
			standIn[ch.ID] = ch.PlaysAs
		}
		ids = append(ids, ch.ID)
	}
	if len(ids) == 0 {
		return []AiringHit{}, nil
	}
	channels, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	// A stand-in's duplicates are dropped below, so read a few more.
	// Title hits come back first. The rest are read only when the title
	// hits do not already fill that slack, so a common word in every
	// description is not sorted ahead of the shows it names.
	slack := limit + len(standIn)*4
	fromArg := from.UTC().Format(time.RFC3339)
	if title == "" {
		hits, err := s.pickPerShow(ctx, slack, searchTitleSQL, match, fromArg, string(channels))
		if err != nil {
			return nil, err
		}
		return withoutTwinRepeats(hits, standIn, limit), nil
	}
	hits, err := s.pickPerShow(ctx, slack, searchTitleSQL, title, fromArg, string(channels))
	if err != nil {
		return nil, err
	}
	if len(hits) < slack {
		rest, err := s.pickPerShow(ctx, slack-len(hits), searchRestSQL, match, fromArg, string(channels), title)
		if err != nil {
			return nil, err
		}
		hits = append(hits, rest...)
	}
	return withoutTwinRepeats(hits, standIn, limit), nil
}

const searchHitColumns = `a.id, a.channel_id, a.title, a.subtitle, a.description, a.category, a.starts_at, a.ends_at,
	a.program_id, a.is_new, a.image_url, a.image_width, a.image_height, a.season, a.episode, a.episode_label, a.original_air, a.series_id,
	a.is_live, a.is_premiere, a.is_finale, a.rating, a.cast_list, a.game_id, c.guide_number, c.guide_name`

// perShow is how many airings of one show a search returns. A two-week guide
// lists a daily show fourteen times and a news word matches hundreds of
// listings; without a cap the next few hours fill the results and a show on
// next week never appears. A game is its own show.
const perShow = 6

const searchWhere = `
FROM airing_search
JOIN airings a ON a.id = airing_search.rowid
WHERE airing_search MATCH ?
  AND a.ends_at > ?
  AND a.channel_id IN (SELECT CAST(value AS INTEGER) FROM json_each(?))`

// The pick reads only what it counts by, soonest first, and stops when it has
// enough; the full rows are read for the ones kept.
const searchTitleSQL = `SELECT a.id, a.title, a.game_id` + searchWhere + `
ORDER BY a.starts_at, a.id`

const searchRestSQL = `SELECT a.id, a.title, a.game_id` + searchWhere + `
  AND a.id NOT IN (SELECT rowid FROM airing_search WHERE airing_search MATCH ?)
ORDER BY a.starts_at, a.id`

const searchRowsSQL = `SELECT ` + searchHitColumns + `
FROM airings a
JOIN channels c ON c.id = a.channel_id
WHERE a.id IN (SELECT CAST(value AS INTEGER) FROM json_each(?))
ORDER BY a.starts_at, a.id`

// pickPerShow keeps the soonest perShow airings of each show, up to want in all.
func (s *Store) pickPerShow(ctx context.Context, want int, sqlText string, args ...any) ([]AiringHit, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, err
	}
	type show struct{ title, game string }
	count := map[show]int{}
	var ids []int64
	for len(ids) < want && rows.Next() {
		var id int64
		var key show
		if err := rows.Scan(&id, &key.title, &key.game); err != nil {
			rows.Close()
			return nil, err
		}
		key.title = strings.ToLower(key.title)
		if count[key] == perShow {
			continue
		}
		count[key]++
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ids) == 0 {
		return nil, nil
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	return s.querySearchHits(ctx, searchRowsSQL, string(list))
}

func (s *Store) querySearchHits(ctx context.Context, sqlText string, args ...any) ([]AiringHit, error) {
	rows, err := s.db.QueryContext(ctx, sqlText, args...)
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
	return out, rows.Err()
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

// teamMatch is an FTS query for listings whose title or subtitle names one
// of the teams. Each name is one quoted phrase, so FTS splits it the way it
// split the listing (accents, "A&M", one-letter words) and no character is
// query syntax. Description and cast are not searched: a news item that
// mentions a club is not that club's game. An empty result means no name had
// a letter or digit.
func teamMatch(names []string) string {
	var parts []string
	for _, name := range names {
		if strings.IndexFunc(name, func(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }) < 0 {
			continue
		}
		phrase := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
		parts = append(parts, `(title : `+phrase+` OR subtitle : `+phrase+`)`)
	}
	return strings.Join(parts, " OR ")
}

// ftsMatch keeps letters and digits in any script. It kept only ASCII, so
// "Pokémon" searched for "Pokmon" and "Café" for "Caf". The index folds
// accents (unicode61 removes diacritics), and the quoted word is folded the
// same way, so "pokemon" and "Pokémon" find each other.
func ftsMatch(query string) string {
	fields := strings.Fields(query)
	var parts []string
	for _, field := range fields {
		var b strings.Builder
		for _, r := range field {
			if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.Is(unicode.Mn, r) || r == '-' {
				b.WriteRune(r)
			}
		}
		word := b.String()
		if utf8.RuneCountInString(word) < 2 {
			continue
		}
		parts = append(parts, `"`+word+`"*`)
	}
	return strings.Join(parts, " ")
}
