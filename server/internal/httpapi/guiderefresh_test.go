package httpapi

import (
	"errors"
	"testing"
	"time"

	"broadwave/internal/guide"
	"broadwave/internal/store"
)

func TestCombineGuide(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	// rows lists n half-hour airings from start; detail fills a description.
	rows := func(source string, ch int64, start time.Duration, n int, detail bool) []store.Airing {
		var out []store.Airing
		for i := range n {
			at := now.Add(start + time.Duration(i)*30*time.Minute)
			row := store.Airing{ChannelID: ch, Title: source, GuideSource: source, Start: at, End: at.Add(30 * time.Minute)}
			if detail {
				row.Description = "More."
			}
			out = append(out, row)
		}
		return out
	}
	count := func(rows []store.Airing, ch int64, source string) int {
		n := 0
		for _, row := range rows {
			if row.ChannelID == ch && row.GuideSource == source {
				n++
			}
		}
		return n
	}
	noOverlap := func(t *testing.T, rows []store.Airing) {
		t.Helper()
		last := map[int64]time.Time{}
		for _, row := range rows {
			if end, ok := last[row.ChannelID]; ok && row.Start.Before(end) {
				t.Fatalf("channel %d: %s at %v overlaps an airing ending %v", row.ChannelID, row.GuideSource, row.Start, end)
			}
			last[row.ChannelID] = row.End
		}
	}

	t.Run("a short feed is extended by a longer one past its end", func(t *testing.T) {
		// Two days of SiliconDust, fourteen of Schedules Direct, equally detailed.
		got := combineGuide([]guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 0, 96, true)},
			{source: "schedules-direct", rows: rows("schedules-direct", 1, 0, 14*48, true)},
		})
		noOverlap(t, got)
		if count(got, 1, "silicondust") != 96 || count(got, 1, "schedules-direct") != 14*48-96 {
			t.Fatalf("silicondust %d, schedules direct %d", count(got, 1, "silicondust"), count(got, 1, "schedules-direct"))
		}
		if end := got[len(got)-1].End; !end.Equal(now.Add(14 * 24 * time.Hour)) {
			t.Fatalf("guide ends %v", end)
		}
	})

	t.Run("the more detailed feed keeps the stretch both list", func(t *testing.T) {
		got := combineGuide([]guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 0, 96, false)},
			{source: "schedules-direct", rows: rows("schedules-direct", 1, 0, 200, true)},
		})
		noOverlap(t, got)
		if count(got, 1, "silicondust") != 0 || count(got, 1, "schedules-direct") != 200 {
			t.Fatalf("silicondust %d, schedules direct %d", count(got, 1, "silicondust"), count(got, 1, "schedules-direct"))
		}
	})

	t.Run("airings on other slot times never overlap", func(t *testing.T) {
		// An XMLTV feed whose airings start on the quarter hour, an hour long.
		var shifted []store.Airing
		for i := range 60 {
			at := now.Add(15*time.Minute + time.Duration(i)*time.Hour)
			shifted = append(shifted, store.Airing{ChannelID: 1, Title: "x", GuideSource: "xmltv", Start: at, End: at.Add(time.Hour)})
		}
		got := combineGuide([]guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 0, 48, true)},
			{source: "xmltv", rows: shifted},
		})
		noOverlap(t, got)
		if count(got, 1, "silicondust") != 48 || count(got, 1, "xmltv") == 0 {
			t.Fatalf("silicondust %d, xmltv %d", count(got, 1, "silicondust"), count(got, 1, "xmltv"))
		}
	})

	t.Run("each channel is combined on its own", func(t *testing.T) {
		got := combineGuide([]guideFeed{
			{source: "silicondust", rows: rows("silicondust", 1, 0, 4, true)},
			{source: "schedules-direct", rows: append(rows("schedules-direct", 1, 0, 9, false), rows("schedules-direct", 2, 0, 9, false)...)},
			{source: "xmltv", rows: rows("xmltv", 3, 0, 5, false)},
		})
		noOverlap(t, got)
		if count(got, 1, "silicondust") != 4 || count(got, 1, "schedules-direct") != 5 || count(got, 2, "schedules-direct") != 9 || count(got, 3, "xmltv") != 5 {
			t.Fatalf("rows %+v", got)
		}
	})

	t.Run("an airing a feed lists twice is kept once", func(t *testing.T) {
		twice := append(rows("xmltv", 1, 0, 4, false), rows("xmltv", 1, 0, 4, false)...)
		if got := combineGuide([]guideFeed{{source: "xmltv", rows: twice}}); len(got) != 4 {
			t.Fatalf("%d rows", len(got))
		}
	})
}

func TestFellTooFar(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	fresh := make([]store.Airing, 20)
	for i := range fresh {
		fresh[i] = store.Airing{Start: now, End: now.Add(time.Hour)}
	}
	if fellTooFar(fresh, 400, now) == nil {
		t.Fatal("a feed down to 20 of 400 listings was taken")
	}
	// A small guide is not judged: a new lineup starts there.
	if err := fellTooFar(fresh, 60, now); err != nil {
		t.Fatal(err)
	}
}

func TestGuideBackoff(t *testing.T) {
	want := []time.Duration{30 * time.Minute, time.Hour, 2 * time.Hour, 4 * time.Hour, 6 * time.Hour, 6 * time.Hour}
	for i, w := range want {
		if got := guideBackoff(i + 1); got != w {
			t.Errorf("failure %d: %v, want %v", i+1, got, w)
		}
	}
	if guide.RetryAfterError != 30*time.Minute {
		t.Fatal("first wait changed")
	}
}

func TestGuideSourceErrorNamesEachSource(t *testing.T) {
	err := &GuideSourceError{Failed: map[string]error{sourceXMLTV: guide.ErrNoPrograms, sourceSilicon: errors.New("guide returned 403 Forbidden")}}
	if err.Error() != "guide returned 403 Forbidden; the XMLTV link gave no programs" || !errors.Is(err, guide.ErrNoPrograms) {
		t.Fatalf("%q", err.Error())
	}
}
