package dvr

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestPlanTunerConflicts(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "News", ChannelID: 0},
		{ID: 2, Title: "Game", ChannelID: 5},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ID: 3, ChannelID: 3, Title: "News", Start: start.Add(30 * time.Minute), End: start.Add(90 * time.Minute)},
		{ID: 4, ChannelID: 1, Title: "News", Start: start.Add(time.Hour), End: start.Add(2 * time.Hour)},
		{ID: 5, ChannelID: 4, Title: "Game", Start: start, End: start.Add(time.Hour)},
		{ID: 6, ChannelID: 9, Title: "Other", Start: start, End: start.Add(time.Hour)},
	}
	from := start.Add(-time.Minute)
	to := start.Add(3 * time.Hour)

	got := Plan(passes, airings, 2, from, to)
	if len(got) != 4 {
		t.Fatalf("len %d", len(got))
	}
	// Game is locked to channel 5, so the channel 4 airing is ignored. Other has no pass.
	skipped := map[int64]bool{}
	for _, item := range got {
		skipped[item.Airing.ID] = item.Skipped
	}
	if skipped[1] || skipped[2] || !skipped[3] {
		t.Fatalf("equal priority keeps the two that started first: %+v", got)
	}
	if skipped[4] {
		t.Fatalf("back-to-back on the same channel should fit: %+v", got)
	}

	pair := Plan(passes, airings[:2], 2, from, to)
	for _, item := range pair {
		if item.Skipped {
			t.Fatalf("two channels fit on two tuners: %+v", pair)
		}
	}
}

func TestPlanKeepsHigherPriority(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "Low", ChannelID: 1, Priority: 1},
		{ID: 2, Title: "High", ChannelID: 2, Priority: 5, PadBefore: 1, PadAfter: 2},
		{ID: 3, Title: "Zero", ChannelID: 3, Priority: 0},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "Low", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "High", Start: start, End: start.Add(time.Hour)},
		{ID: 3, ChannelID: 3, Title: "Zero", Start: start, End: start.Add(time.Hour)},
	}
	got := Plan(passes, airings, 2, start.Add(-time.Minute), start.Add(2*time.Hour))
	kept := map[int64]bool{}
	for _, item := range got {
		kept[item.Airing.ID] = !item.Skipped
	}
	if !kept[1] || !kept[2] || kept[3] {
		t.Fatalf("priority 0 gives up the tuner: %+v", got)
	}
	for _, item := range got {
		if item.Airing.ID == 2 && (item.PadBefore != 1 || item.PadAfter != 2) {
			t.Fatalf("padding should travel with the pass: %+v", item)
		}
	}
	one := Plan(passes, airings, 1, start.Add(-time.Minute), start.Add(2*time.Hour))
	for _, item := range one {
		if item.Airing.ID == 2 && item.Skipped {
			t.Fatalf("highest priority should be the one that records: %+v", one)
		}
		if item.Airing.ID != 2 && !item.Skipped {
			t.Fatalf("lower priorities wait: %+v", one)
		}
	}
}

func TestOncePassRecordsOnlyItsAiring(t *testing.T) {
	start := time.Date(2026, 9, 28, 16, 0, 0, 0, time.UTC)
	passes := []store.Pass{{ID: 1, Title: "Jeopardy", ChannelID: 3, Kind: "once", AiringStart: start}}
	airings := []store.Airing{
		{ID: 1, ChannelID: 3, Title: "Jeopardy!", ProgramID: "EP1", Start: start, End: start.Add(30 * time.Minute)},
		{ID: 2, ChannelID: 3, Title: "Jeopardy", Start: start.Add(24 * time.Hour), End: start.Add(24*time.Hour + 30*time.Minute)},
		{ID: 3, ChannelID: 4, Title: "Jeopardy", Start: start, End: start.Add(30 * time.Minute)},
	}
	// A series pass for the same show comes first by title and would call it already recorded.
	passes = append([]store.Pass{{ID: 2, Title: "Jeopardy!", ChannelID: 3}}, passes...)
	got := Plan(passes, airings, 2, start.Add(-time.Hour), start.Add(48*time.Hour))
	if len(got) < 1 || got[0].Airing.ID != 1 || got[0].PassID != 1 {
		t.Fatalf("the once pass owns its airing: %+v", got)
	}
	for _, item := range got[1:] {
		if item.PassID == 1 {
			t.Fatalf("a once pass records the retitled airing at its start and nothing else: %+v", got)
		}
	}
	seen := map[string]bool{store.EpisodeKey("EP1", "Jeopardy!", "", 3): false}
	got = ApplyLibrary(got, passes, nil, seen, nil)
	if got[0].Skipped {
		t.Fatalf("asked for by name, it records even if seen before: %+v", got[0])
	}
}
