package store

import (
	"context"
	"slices"
	"testing"
	"time"
)

func TestReplaceAiringsForReplacesChannelListings(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	at := func(h, m int) time.Time { return time.Date(2026, 9, 27, h, m, 0, 0, time.UTC) }
	row := func(ch int64, title, source string, start time.Time, mins int) Airing {
		return Airing{ChannelID: ch, Title: title, GuideSource: source, Start: start, End: start.Add(time.Duration(mins) * time.Minute)}
	}
	if err := st.InsertAirings(ctx, []Airing{
		row(1, "Late film", "broadcast", at(9, 0), 60),
		row(1, "Overlapped", "broadcast", at(7, 0), 30),
	}); err != nil {
		t.Fatal(err)
	}
	first := []Airing{row(1, "Jeopardy!", "silicondust", at(7, 0), 30), row(2, "News", "silicondust", at(7, 0), 30)}
	if err := st.ReplaceAiringsFor(ctx, []int64{1, 2}, first); err != nil {
		t.Fatal(err)
	}
	// The next pull overlaps the last one, as the 2-day feed does every day.
	next := []Airing{row(1, "Jeopardy!", "silicondust", at(7, 0), 30), row(1, "Wheel", "silicondust", at(7, 30), 30)}
	if err := st.ReplaceAiringsFor(ctx, []int64{1}, next); err != nil {
		t.Fatal(err)
	}
	got, err := st.Airings(ctx, at(0, 0), at(23, 0))
	if err != nil {
		t.Fatal(err)
	}
	var titles []string
	for _, a := range got {
		titles = append(titles, a.Title)
	}
	slices.Sort(titles)
	want := []string{"Jeopardy!", "Late film", "News", "Wheel"}
	if !slices.Equal(titles, want) {
		t.Fatalf("airings = %v, want %v", titles, want)
	}
}

func TestReplaceAiringsForKeepsOneRowPerSlot(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	start := time.Date(2026, 9, 27, 7, 0, 0, 0, time.UTC)
	bare := Airing{ChannelID: 1, Title: "Jeopardy!", Start: start, End: start.Add(30 * time.Minute)}
	rich := bare
	rich.Description = "A classic game show."
	if err := st.ReplaceAiringsFor(ctx, []int64{1}, []Airing{bare, rich}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Airings(ctx, start.Add(-time.Hour), start.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Description != rich.Description {
		t.Fatalf("airings = %+v, want the one with a description", got)
	}
}
