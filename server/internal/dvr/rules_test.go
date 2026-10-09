package dvr

import (
	"fmt"
	"testing"
	"time"

	"broadwave/internal/store"
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
		{ID: 2, Title: "bears", MatchKind: "contains", ChannelID: 4},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 2, Title: "Noon news", Category: "News, Sports", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 4, Title: "Bears at Broncos", Start: start, End: start.Add(3 * time.Hour)},
		{ID: 3, ChannelID: 9, Title: "Bears wrap", Start: start, End: start.Add(time.Hour)},
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

// A copy the signal ruined does not stop the next airing; a clean one does.
func TestADamagedCopyRecordsAgain(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	pass := store.Pass{ID: 1, Title: "Show"}
	airing := store.Airing{ID: 9, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Start: start, End: start.Add(time.Hour)}
	key := store.EpisodeKey("EP1", "Show", "Pilot", 1)
	seen := map[string]bool{key: false}
	broken := &store.RecordingHealth{LostSeconds: 40}
	broken.Judge(1800)
	recs := []store.Recording{{ID: 3, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "complete", Health: broken}}
	if got := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, recs, seen, nil); got[0].Skipped {
		t.Fatalf("damaged copy: %+v", got[0])
	}
	clean := append(recs, store.Recording{ID: 4, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "complete", Health: &store.RecordingHealth{}})
	if got := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, clean, seen, nil); !got[0].Skipped {
		t.Fatalf("clean copy: %+v", got[0])
	}
	// A second damaged copy: the station comes in that way, so stop.
	twice := append(recs, store.Recording{ID: 5, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "complete", Health: broken})
	if got := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, twice, seen, nil); !got[0].Skipped {
		t.Fatalf("two damaged copies: %+v", got[0])
	}
}

// Starting a recording remembers the episode before the file is open. A failed
// row plus that flag is not a copy, so the next airing still records.
func TestAFailedStartRecordsAgain(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	pass := store.Pass{ID: 1, Title: "Show"}
	airing := store.Airing{ID: 9, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Start: start, End: start.Add(time.Hour)}
	key := store.EpisodeKey("EP1", "Show", "Pilot", 1)
	seen := map[string]bool{key: false}
	failed := []store.Recording{{ID: 3, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "failed"}}
	if got := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, failed, seen, nil); got[0].Skipped {
		t.Fatalf("failed start: %+v", got[0])
	}
	done := []store.Recording{{ID: 4, ChannelID: 1, Title: "Show", Subtitle: "Pilot", ProgramID: "EP1", Status: "complete"}}
	if got := ApplyLibrary([]Planned{{PassID: 1, Airing: airing}}, []store.Pass{pass}, done, seen, nil); !got[0].Skipped {
		t.Fatalf("finished copy: %+v", got[0])
	}
}

func TestDamagedIsLostTimeOrManyBreakups(t *testing.T) {
	for _, c := range []struct {
		h      store.RecordingHealth
		length float64
		want   bool
	}{
		{store.RecordingHealth{}, 1800, false},
		{store.RecordingHealth{ContinuityErrors: 12, Gaps: 1, LostSeconds: 2.5}, 1800, false},
		{store.RecordingHealth{Gaps: 2, LostSeconds: 6}, 1800, true},
		{store.RecordingHealth{ContinuityErrors: 60}, 1800, true},
		// Packets in error are not breakups: one fade flags hundreds.
		{store.RecordingHealth{ContinuityErrors: 5, TransportErrors: 400}, 1800, false},
		// A long game breaks up more before it counts as damaged.
		{store.RecordingHealth{ContinuityErrors: 100}, 4 * 3600, false},
	} {
		c.h.Judge(c.length)
		if c.h.Damaged != c.want {
			t.Fatalf("%+v", c.h)
		}
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

func TestKeepRulesCoverOnlyWhatAPassRecorded(t *testing.T) {
	day := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	rec := func(id int64, pass int64, title, category string, daysAgo int) store.Recording {
		return store.Recording{ID: id, PassID: pass, Title: title, Category: category, Status: "complete", StartedAt: day.AddDate(0, 0, -daysAgo)}
	}
	recs := []store.Recording{
		rec(1, 7, "Bears at Bills", "Sports", 3),
		rec(2, 7, "Chiefs at Raiders", "Sports", 1),
		rec(3, 0, "Lakers at Celtics", "Sports", 5), // recorded by hand, or a library file
		rec(4, 9, "Royals at Twins", "Sports", 6),   // another pass's
		rec(5, 0, "News", "News", 4),                // a title pass's, before recordings named their pass
		rec(6, 0, "News", "News", 2),
		rec(7, 7, "News", "Sports", 8),
	}
	sports := store.Pass{ID: 7, Title: "Sports", MatchKind: "category", KeepMode: "last", KeepCount: 1}
	if got := KeepVictims(sports, recs); fmt.Sprint(got) != "[1 7]" {
		t.Fatalf("category pass removes %v, want only its own older ones [1 7]", got)
	}
	news := store.Pass{ID: 8, Title: "News", KeepMode: "last", KeepCount: 1}
	if got := KeepVictims(news, recs); fmt.Sprint(got) != "[5]" {
		t.Fatalf("title pass removes %v, want the older untagged News [5]", got)
	}
	words := store.Pass{ID: 10, Title: "at", MatchKind: "contains", KeepMode: "unwatched", LimitCount: 1}
	if got := KeepVictims(words, recs); len(got) != 0 {
		t.Fatalf("a words pass that recorded nothing removes %v", got)
	}
	if n := unwatchedCount(recs, words); n != 0 {
		t.Fatalf("a words pass that recorded nothing counts %d toward its limit", n)
	}
}

func TestAnAiringThatWontRecordLeavesItsTuner(t *testing.T) {
	start := time.Date(2026, 9, 22, 19, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "Rerun", Priority: 5},
		{ID: 2, Title: "News", Priority: 1},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "Rerun", ProgramID: "EP1", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "News", Start: start, End: start.Add(time.Hour)},
	}
	recs := []store.Recording{{ID: 1, Title: "Rerun", ProgramID: "EP1", Status: "complete", PassID: 1}}
	got := PlanLibrary(passes, airings, 1, start.Add(-time.Minute), start.Add(2*time.Hour), recs, nil, nil)
	if len(got) != 2 || !got[0].Skipped || got[0].Reason != "Already recorded" || got[1].Skipped {
		t.Fatalf("the recorded rerun held the tuner: %+v", got)
	}
}

func TestAKeepRuleLeavesFilesOutsideTheRecordingsFolder(t *testing.T) {
	root := "/config/work/recordings"
	for path, want := range map[string]bool{
		"/config/work/recordings/a.ts":     true,
		"/config/work/recordings/sub/a.ts": true,
		"/media/library/a.ts":              false,
		"/config/work/recordings/../a.ts":  false,
		"/config/work/recordings":          false,
		"":                                 false,
	} {
		if got := insideDir(root, path); got != want {
			t.Errorf("%q: %v, want %v", path, got, want)
		}
	}
}

func TestKeepRulesSkipAKeptRecording(t *testing.T) {
	pass := store.Pass{Title: "News", KeepMode: "last", KeepCount: 1}
	day := time.Date(2026, 10, 1, 18, 0, 0, 0, time.UTC)
	recs := []store.Recording{
		{ID: 1, Title: "News", Status: "complete", StartedAt: day, Keep: true},
		{ID: 2, Title: "News", Status: "complete", StartedAt: day.Add(24 * time.Hour)},
		{ID: 3, Title: "News", Status: "complete", StartedAt: day.Add(48 * time.Hour)},
	}
	if got := KeepVictims(pass, recs); fmt.Sprint(got) != "[2]" {
		t.Fatalf("last 1: %v", got)
	}
	watched := store.Pass{Title: "News", KeepMode: "unwatched"}
	recs[0].Watched = 1
	if got := KeepVictims(watched, recs); len(got) != 0 {
		t.Fatalf("unwatched: %v", got)
	}
}
