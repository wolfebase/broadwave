package dvr

import (
	"testing"
	"time"

	"ota-viewer/internal/store"
)

func TestNewOnlySkipsReruns(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{{ID: 1, Title: "Jeopardy!", Episodes: "new", Commercials: true}}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "Jeopardy!", New: false, Start: start, End: start.Add(30 * time.Minute)},
		{ID: 2, ChannelID: 1, Title: "Jeopardy!", New: true, Start: start.Add(time.Hour), End: start.Add(90 * time.Minute)},
	}
	got := Plan(passes, airings, 2, start.Add(-time.Minute), start.Add(3*time.Hour))
	if len(got) != 1 || got[0].Airing.ID != 2 {
		t.Fatalf("new only: %+v", got)
	}
}

func TestCategoryAndContains(t *testing.T) {
	start := time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "Sports", MatchKind: "category"},
		{ID: 2, Title: "chiefs", MatchKind: "contains", ChannelID: 4},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 2, Title: "Noon news", Category: "News, Sports", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 4, Title: "Chiefs at Broncos", Start: start, End: start.Add(3 * time.Hour)},
		{ID: 3, ChannelID: 9, Title: "Chiefs wrap", Start: start, End: start.Add(time.Hour)},
	}
	got := Plan(passes, airings, 2, start.Add(-time.Minute), start.Add(4*time.Hour))
	ids := map[int64]bool{}
	for _, item := range got {
		ids[item.Airing.ID] = true
	}
	if !ids[1] || !ids[2] || ids[3] {
		t.Fatalf("category and team: %+v", got)
	}
}

func TestSkipDuplicateUnlessRerecord(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	pass := store.Pass{ID: 1, Title: "Show", Commercials: true}
	airing := store.Airing{ID: 9, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Start: start, End: start.Add(time.Hour)}
	items := []Planned{{PassID: 1, Airing: airing}}
	recs := []store.Recording{{ID: 3, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "complete"}}
	got := ApplyLibrary(items, []store.Pass{pass}, recs, nil, nil)
	if !got[0].Skipped || got[0].Reason != "Already recorded" {
		t.Fatalf("duplicate: %+v", got[0])
	}
	pass.Rerecord = true
	seen := map[string]bool{store.EpisodeKey("EP1", "Show", "Pilot", 1): true}
	again := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, nil, seen, nil)
	if again[0].Skipped {
		t.Fatalf("rerecord should allow a deleted episode: %+v", again[0])
	}
}

func TestKeepLast(t *testing.T) {
	pass := store.Pass{Title: "News", KeepMode: "last", KeepCount: 2}
	start := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	recs := []store.Recording{
		{ID: 1, Title: "News", Status: "complete", StartedAt: start},
		{ID: 2, Title: "News", Status: "complete", StartedAt: start.Add(time.Hour)},
		{ID: 3, Title: "News", Status: "complete", StartedAt: start.Add(2 * time.Hour)},
		{ID: 4, Title: "News", Status: "recording", StartedAt: start.Add(3 * time.Hour)},
	}
	got := KeepVictims(pass, recs)
	if len(got) != 1 || got[0] != 1 {
		t.Fatalf("keep last 2, drop the oldest finished: %v", got)
	}
}
