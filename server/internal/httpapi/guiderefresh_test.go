package httpapi

import (
	"errors"
	"slices"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestMergeGuide(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	rows := func(source string, ch int64, n int) []store.Airing {
		var out []store.Airing
		for i := range n {
			at := now.Add(time.Duration(i) * 30 * time.Minute)
			out = append(out, store.Airing{ChannelID: ch, Title: source, GuideSource: source, Start: at, End: at.Add(30 * time.Minute)})
		}
		return out
	}
	channels := []int64{1, 2, 3}
	count := func(m guideMerge, ch int64, source string) int {
		n := 0
		for _, row := range m.rows {
			if row.ChannelID == ch && row.GuideSource == source {
				n++
			}
		}
		return n
	}

	t.Run("earlier feed wins a channel, later ones fill the rest", func(t *testing.T) {
		m := mergeGuide(channels, []guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 4)},
			{source: "schedules-direct", rows: append(rows("schedules-direct", 1, 9), rows("schedules-direct", 2, 9)...)},
			{source: "xmltv", off: true},
		}, store.GuideCoverage{}, now)
		if count(m, 1, "silicondust") != 4 || count(m, 1, "schedules-direct") != 0 || count(m, 2, "schedules-direct") != 9 {
			t.Fatalf("rows %+v", m.rows)
		}
		if !slices.Equal(m.replace, channels) {
			t.Fatalf("replace %v", m.replace)
		}
	})

	t.Run("a failed feed keeps its channels", func(t *testing.T) {
		prev := store.GuideCoverage{Rows: map[string]int{"silicondust": 40, "schedules-direct": 300}, Channels: map[int64]string{1: "silicondust", 2: "schedules-direct", 3: "schedules-direct"}}
		m := mergeGuide(channels, []guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 4)},
			{source: "schedules-direct", err: errors.New("schedules direct returned 503")},
		}, prev, now)
		if !slices.Equal(m.replace, []int64{1}) || m.held["schedules-direct"] == "" {
			t.Fatalf("replace %v held %v", m.replace, m.held)
		}
	})

	t.Run("a feed whose rows fall by more than half is held", func(t *testing.T) {
		prev := store.GuideCoverage{Rows: map[string]int{"silicondust": 400}, Channels: map[int64]string{1: "silicondust", 2: "silicondust", 3: "silicondust"}}
		m := mergeGuide(channels, []guideFeed{{source: "silicondust", rows: rows("silicondust", 1, 20)}}, prev, now)
		if len(m.rows) != 0 || len(m.replace) != 0 || m.held["silicondust"] == "" {
			t.Fatalf("rows %d replace %v held %v", len(m.rows), m.replace, m.held)
		}
		// A small guide is not judged: a new lineup starts there.
		prev.Rows["silicondust"] = 60
		m = mergeGuide(channels, []guideFeed{{source: "silicondust", rows: rows("silicondust", 1, 20)}}, prev, now)
		if len(m.rows) != 20 || len(m.held) != 0 {
			t.Fatalf("small guide held: %v", m.held)
		}
	})

	t.Run("a held feed's channel goes to an earlier feed that lists it now", func(t *testing.T) {
		prev := store.GuideCoverage{Rows: map[string]int{"xmltv": 10}, Channels: map[int64]string{2: "xmltv", 3: "xmltv"}}
		m := mergeGuide(channels, []guideFeed{
			{source: "silicondust", rows: rows("silicondust", 2, 4)},
			{source: "xmltv", err: errors.New("guide returned 500")},
		}, prev, now)
		if !slices.Equal(m.replace, []int64{1, 2}) {
			t.Fatalf("replace %v", m.replace)
		}
	})
}
