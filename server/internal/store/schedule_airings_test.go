package store

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestExactPassesSkipTheRestOfTheGuide(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)
	later := start.Add(24 * time.Hour)
	if err := st.InsertAirings(ctx, []Airing{
		{ChannelID: 1, Title: "Cedar Quiz", Category: "Series", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 2, Title: "cedar quiz", Category: "Series", Start: later, End: later.Add(time.Hour)},
		{ChannelID: 3, Title: "Harbor News", Category: "News", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 4, Title: "Cedar Quiz Special", Category: "Series", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 5, Title: "Cedar Quiz Special", Category: "Series", Start: later, End: later.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	from, to := start.Add(-time.Minute), start.Add(14*24*time.Hour)
	rows, err := st.RecordingAiringsFor(ctx, from, to, []Pass{
		{ID: 1, Title: "  Cedar Quiz ", Kind: "series", MatchKind: "title", ChannelID: 1},
		{ID: 2, Title: "cedar quiz", Kind: "series"},
		{ID: 3, Title: "Old name", Kind: "once", ChannelID: 4, AiringStart: start},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, row := range rows {
		got[row.Title]++
	}
	if got["Cedar Quiz"] != 1 || got["cedar quiz"] != 1 || got["Cedar Quiz Special"] != 2 || got["Harbor News"] != 0 {
		t.Fatalf("rows %v", got)
	}
	if len(rows) != 4 {
		t.Fatalf("len %d", len(rows))
	}
}

func TestLoosePassesStillReadTheWindow(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 10, 9, 20, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []Airing{
		{ChannelID: 1, Title: "Harbor News", Category: "News", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 2, Title: "café hour", Category: "Series", Start: start, End: start.Add(time.Hour)},
		{ChannelID: 3, Title: "Harbor at Cedar", Subtitle: "Wolves", Category: "Sports", Start: start, End: start.Add(3 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	from, to := start.Add(-time.Minute), start.Add(24*time.Hour)
	for _, pass := range []Pass{
		{Title: "Sports", Kind: "series", MatchKind: "category"},
		{Title: "Quiz", Kind: "series", MatchKind: "contains"},
		{Title: "Wolves", Kind: "team"},
		{Title: "Café Hour", Kind: "series", MatchKind: "title"},
	} {
		rows, err := st.RecordingAiringsFor(ctx, from, to, []Pass{pass})
		if err != nil {
			t.Fatal(err)
		}
		sawNews := false
		for _, row := range rows {
			if row.Title == "Harbor News" {
				sawNews = true
			}
		}
		if !sawNews || len(rows) != 3 {
			t.Fatalf("pass %+v rows %d news %v", pass, len(rows), sawNews)
		}
	}
	none, err := st.RecordingAiringsFor(ctx, from, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Fatalf("no passes returned %d", len(none))
	}
}

func TestTitleAndOncePlansUseTheirIndexes(t *testing.T) {
	st := openTestStore(t)
	from := time.Date(2026, 10, 9, 18, 0, 0, 0, time.UTC)
	to := from.Add(14 * 24 * time.Hour)
	q := AiringQuery{From: from, To: to, Titles: []string{"Cedar Quiz"}}
	sqlText, args, ok := airingSelect(q)
	if !ok {
		t.Fatal("expected a query")
	}
	plan := queryPlan(t, st, sqlText, args...)
	if !strings.Contains(plan, "airings_title_start") || strings.Contains(plan, "airings_starts") {
		t.Fatalf("title plan:\n%s", plan)
	}
	start := from.UTC().Format(time.RFC3339)
	if plan := queryPlan(t, st, airingAtSelect(), int64(4), start); !strings.Contains(plan, "airings_channel_start") {
		t.Fatalf("once plan:\n%s", plan)
	}
	open := AiringQuery{From: from, To: to}
	sqlText, args, ok = airingSelect(open)
	if !ok {
		t.Fatal("expected a query")
	}
	plan = queryPlan(t, st, sqlText, args...)
	if !strings.Contains(plan, "airings_starts") {
		t.Fatalf("open window left the start index:\n%s", plan)
	}
}
