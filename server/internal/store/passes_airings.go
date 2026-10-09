package store

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// RecordingAiringsFor is RecordingAirings limited to the listings these passes
// can match. An exact title or a once pass names its rows, on every channel,
// so a later showing can still be offered. A category, a word, a team, or a
// title with a non-ASCII letter still reads the whole window: those matches
// are not one title, and SQLite's NOCASE fold covers ASCII letters only.
func (s *Store) RecordingAiringsFor(ctx context.Context, from, to time.Time, passes []Pass) ([]Airing, error) {
	if len(passes) == 0 {
		return []Airing{}, nil
	}
	if !exactPassesOnly(passes) {
		return s.RecordingAirings(ctx, from, to)
	}
	rows, wide, err := s.exactPassAirings(ctx, from, to, passes)
	if err != nil {
		return nil, err
	}
	if wide {
		return s.RecordingAirings(ctx, from, to)
	}
	return s.markSimulcasts(ctx, rows)
}

func exactPassesOnly(passes []Pass) bool {
	for _, pass := range passes {
		if passNeedsWindow(pass) {
			return false
		}
	}
	return true
}

// passNeedsWindow reports a pass whose match is not an exact ASCII title and
// not a single channel and start. A once pass ignores its title: the guide
// can retitle that airing.
func passNeedsWindow(pass Pass) bool {
	if strings.EqualFold(strings.TrimSpace(pass.Kind), "once") {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(pass.Kind), "team") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(pass.MatchKind)) {
	case "category", "contains", "team":
		return true
	default:
		return !asciiTitle(pass.Title)
	}
}

func asciiTitle(title string) bool {
	for _, r := range title {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// exactPassAirings reads the rows an exact title or a once pass can name.
// wide is set when a once airing's title is not ASCII, so the caller has to
// read the window to find its later showings.
func (s *Store) exactPassAirings(ctx context.Context, from, to time.Time, passes []Pass) ([]Airing, bool, error) {
	seenTitle := map[string]bool{}
	var titles []string
	seenSlot := map[string]bool{}
	type slot struct {
		channel int64
		start   time.Time
	}
	var slots []slot
	for _, pass := range passes {
		if strings.EqualFold(strings.TrimSpace(pass.Kind), "once") {
			if pass.ChannelID == 0 {
				continue
			}
			key := strconv.FormatInt(pass.ChannelID, 10) + "|" + pass.AiringStart.UTC().Format(time.RFC3339)
			if seenSlot[key] {
				continue
			}
			seenSlot[key] = true
			slots = append(slots, slot{pass.ChannelID, pass.AiringStart})
			continue
		}
		title := strings.TrimSpace(pass.Title)
		key := strings.ToLower(title)
		if seenTitle[key] {
			continue
		}
		seenTitle[key] = true
		titles = append(titles, title)
	}
	var rows []Airing
	if len(titles) > 0 {
		got, err := s.QueryAirings(ctx, AiringQuery{From: from, To: to, Titles: titles})
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, got...)
	}
	var extra []string
	for _, sl := range slots {
		got, err := s.airingsAt(ctx, sl.channel, sl.start)
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, got...)
		for _, row := range got {
			title := strings.TrimSpace(row.Title)
			key := strings.ToLower(title)
			if seenTitle[key] {
				continue
			}
			if !asciiTitle(title) {
				return nil, true, nil
			}
			seenTitle[key] = true
			extra = append(extra, title)
		}
	}
	if len(extra) > 0 {
		got, err := s.QueryAirings(ctx, AiringQuery{From: from, To: to, Titles: extra})
		if err != nil {
			return nil, false, err
		}
		rows = append(rows, got...)
	}
	return dedupeAirings(rows), false, nil
}

func (s *Store) airingsAt(ctx context.Context, channelID int64, start time.Time) ([]Airing, error) {
	rows, err := s.db.QueryContext(ctx, airingAtSelect(), channelID, start.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanAirings(rows)
}

func dedupeAirings(rows []Airing) []Airing {
	seen := make(map[int64]struct{}, len(rows))
	out := make([]Airing, 0, len(rows))
	for _, row := range rows {
		if row.ID != 0 {
			if _, ok := seen[row.ID]; ok {
				continue
			}
			seen[row.ID] = struct{}{}
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start.Equal(out[j].Start) {
			return out[i].ID < out[j].ID
		}
		return out[i].Start.Before(out[j].Start)
	})
	return out
}
