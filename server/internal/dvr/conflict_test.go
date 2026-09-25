package dvr

import (
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestSkippedShowSuggestsALaterAiring(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "News", Priority: 1},
		{ID: 2, Title: "Game", ChannelID: 2, Priority: 5},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "Game", Start: start, End: start.Add(time.Hour)},
		{ID: 3, ChannelID: 1, Title: "news", Start: start.Add(2 * time.Hour), End: start.Add(3 * time.Hour)},
	}
	from := start.Add(-time.Minute)
	to := start.Add(5 * time.Hour)
	items := AttachSuggestions(Plan(passes, airings, 1, from, to), passes, airings, 1, from, to, map[int64]string{1: "4.1"})
	var skipped *Planned
	for i := range items {
		if items[i].Airing.ID == 3 && items[i].Skipped {
			t.Fatalf("later airing should record: %+v", items[i])
		}
		if items[i].Airing.ID == 1 {
			skipped = &items[i]
		}
	}
	if skipped == nil || !skipped.Skipped || !skipped.Conflict || skipped.Suggestion == nil {
		t.Fatalf("missing suggestion: %+v", items)
	}
	got := skipped.Suggestion
	if got.ChannelID != 1 || got.GuideNumber != "4.1" || got.Title != "news" || !got.Start.Equal(airings[2].Start) || !got.End.Equal(airings[2].End) {
		t.Fatalf("%+v", got)
	}
	fix := PlanFix(passes[0], skipped.Airing, airings[2])
	if fix.SetChannel != 0 || fix.OneShot != nil {
		t.Fatalf("pass already covers the later airing: %+v", fix)
	}
}

func TestLaterAiringThatStillConflictsIsNotSuggested(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{
		{ID: 1, Title: "News", Priority: 1},
		{ID: 2, Title: "Game", ChannelID: 2, Priority: 5},
	}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "Game", Start: start, End: start.Add(3 * time.Hour)},
		{ID: 3, ChannelID: 3, Title: "News", Start: start.Add(time.Hour), End: start.Add(2 * time.Hour)},
	}
	from := start.Add(-time.Minute)
	to := start.Add(5 * time.Hour)
	items := AttachSuggestions(Plan(passes, airings, 1, from, to), passes, airings, 1, from, to, nil)
	for _, item := range items {
		if item.Suggestion != nil {
			t.Fatalf("conflicting airing suggested: %+v", item)
		}
	}
}

func TestSuggestionCanMoveAChannelPin(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	passes := []store.Pass{{ID: 1, Title: "News", ChannelID: 1, Priority: 1, MatchKind: "title"}}
	game := store.Pass{ID: 2, Title: "Game", ChannelID: 2, Priority: 5}
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "Game", Start: start, End: start.Add(time.Hour)},
		{ID: 3, ChannelID: 3, Title: "News", Start: start.Add(2 * time.Hour), End: start.Add(3 * time.Hour)},
	}
	from := start.Add(-time.Minute)
	to := start.Add(5 * time.Hour)
	items := AttachSuggestions(Plan([]store.Pass{passes[0], game}, airings, 1, from, to), []store.Pass{passes[0], game}, airings, 1, from, to, map[int64]string{3: "5.1"})
	var skipped *Planned
	for i := range items {
		if items[i].Airing.ID == 1 {
			skipped = &items[i]
		}
	}
	if skipped == nil || skipped.Suggestion == nil || skipped.Suggestion.ChannelID != 3 || skipped.Suggestion.GuideNumber != "5.1" || skipped.Suggestion.Misses != nil {
		t.Fatalf("%+v", items)
	}
	fix := PlanFix(passes[0], skipped.Airing, airings[2])
	if fix.SetChannel != 3 || fix.OneShot != nil || fix.Skip.ID != 1 {
		t.Fatalf("%+v", fix)
	}
}

func TestOneShotWhenThePassCannotNameOneAiring(t *testing.T) {
	start := time.Date(2026, 9, 22, 20, 0, 0, 0, time.UTC)
	pass := store.Pass{ID: 4, Title: "Sports", ChannelID: 1, MatchKind: "category", Priority: 2, PadBefore: 1, PadAfter: 2}
	suggestion := store.Airing{ID: 9, ChannelID: 3, Title: "News", Category: "Sports", Start: start.Add(2 * time.Hour), End: start.Add(3 * time.Hour)}
	fix := PlanFix(pass, store.Airing{ID: 1, ChannelID: 1, Title: "News", Start: start}, suggestion)
	if fix.SetChannel != 0 || fix.OneShot == nil {
		t.Fatalf("%+v", fix)
	}
	shot := *fix.OneShot
	if shot.Priority != pass.Priority || shot.ChannelID != 3 || shot.Title != "News" || shot.LimitCount != 1 {
		t.Fatalf("%+v", shot)
	}
	if !passMatches(shot, suggestion) {
		t.Fatalf("one-shot missed the airing: %+v", shot)
	}
	other := suggestion
	other.Start = suggestion.Start.Add(time.Hour)
	other.End = suggestion.End.Add(time.Hour)
	if passMatches(shot, other) {
		t.Fatalf("one-shot matched a different hour: %+v", shot)
	}
	if n := OneShotLimit(shot, []store.Recording{{ChannelID: 3, Title: "News", Status: "complete"}}); n != 2 {
		t.Fatalf("limit %d", n)
	}
}

func TestLiveWatchTunerPad(t *testing.T) {
	now := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	soon := Soon{Title: "News", ChannelID: 5, Start: now.Add(30 * time.Second), Pad: 2 * time.Minute}
	outside := Soon{Title: "News", ChannelID: 5, Start: now.Add(2 * time.Hour), Pad: time.Minute}
	other := Soon{Title: "Game", ChannelID: 8, Start: now.Add(time.Minute), Pad: 2 * time.Minute}

	allow, warning := LiveWatch(1, 0, []Soon{soon}, now)
	if allow || !strings.Contains(warning, "News") || !strings.Contains(warning, "will miss that recording") {
		t.Fatalf("allow=%v %q", allow, warning)
	}
	if allow, warning = LiveWatch(1, 0, []Soon{outside}, now); !allow || warning != "" {
		t.Fatalf("outside the pad: allow=%v %q", allow, warning)
	}
	if allow, _ = LiveWatch(2, 0, []Soon{soon}, now); !allow {
		t.Fatal("a second tuner should cover the recording")
	}
	if allow, _ = LiveWatch(2, 1, []Soon{soon}, now); allow {
		t.Fatal("the busy tuner leaves the recording short")
	}
	if allow, _ = LiveWatch(2, 0, []Soon{soon, other}, now); allow {
		t.Fatal("two recordings need both tuners")
	}
	same := other
	same.ChannelID = soon.ChannelID
	if allow, _ = LiveWatch(2, 0, []Soon{soon, same}, now); !allow {
		t.Fatal("one channel should take one tuner")
	}
}

func TestOneShotNamesTheShowingsItWillMiss(t *testing.T) {
	start := time.Date(2026, 6, 15, 20, 0, 0, 0, time.Local)
	passes := []store.Pass{
		{ID: 4, Title: "Sports", ChannelID: 1, MatchKind: "category", Priority: 1},
		{ID: 2, Title: "Game", ChannelID: 2, Priority: 5},
	}
	later := start.Add(2 * time.Hour)
	again := later.Add(24 * time.Hour)
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "News", Category: "Sports", Start: start, End: start.Add(time.Hour)},
		{ID: 2, ChannelID: 2, Title: "Game", Start: start, End: start.Add(time.Hour)},
		{ID: 3, ChannelID: 3, Title: "News", Category: "Sports", Start: later, End: later.Add(time.Hour)},
		{ID: 4, ChannelID: 3, Title: "News", Category: "Sports", Start: again, End: again.Add(time.Hour)},
	}
	from := start.Add(-time.Minute)
	to := again.Add(time.Hour)
	items := AttachSuggestions(Plan(passes, airings, 1, from, to), passes, airings, 1, from, to, map[int64]string{3: "9.1"})
	var skipped *Planned
	for i := range items {
		if items[i].Airing.ID == 1 {
			skipped = &items[i]
		}
	}
	if skipped == nil || skipped.Suggestion == nil || len(skipped.Suggestion.Misses) != 1 {
		t.Fatalf("%+v", items)
	}
	miss := skipped.Suggestion.Misses[0]
	if miss.ChannelID != 3 || miss.GuideNumber != "9.1" || miss.Title != "News" || !miss.Start.Equal(again) {
		t.Fatalf("%+v", miss)
	}
	line := MissedLine(skipped.Suggestion.Misses)
	want := "News at " + again.Format("3:04 PM") + " on 9.1 will not record."
	if line != want {
		t.Fatalf("%q", line)
	}
	two := MissedLine([]MissedShowing{
		{Title: "News", Start: later, GuideNumber: "9.1"},
		{Title: "Game", Start: later.Add(time.Hour), GuideNumber: "4.1"},
		{Title: " ", Start: again},
	})
	if !strings.Contains(two, "will not record.") || !strings.Contains(two, " and ") || !strings.Contains(two, "A show") {
		t.Fatalf("%q", two)
	}
}

func TestLiveWatchPreemptWindow(t *testing.T) {
	now := time.Date(2026, 9, 25, 18, 0, 0, 0, time.UTC)
	inside := Soon{Title: "News", ChannelID: 5, Start: now.Add(preemptWindow), Pad: 0}
	outside := Soon{Title: "News", ChannelID: 5, Start: now.Add(preemptWindow + time.Millisecond), Pad: 0}
	padded := Soon{Title: "News", ChannelID: 5, Start: now.Add(90 * time.Second), Pad: 2 * time.Minute}

	allow, warning := LiveWatch(1, 0, []Soon{inside}, now)
	if allow || !strings.Contains(warning, "will miss that recording") {
		t.Fatalf("20s window: allow=%v %q", allow, warning)
	}
	if allow, warning = LiveWatch(1, 0, []Soon{outside}, now); !allow || warning != "" {
		t.Fatalf("just outside the window: allow=%v %q", allow, warning)
	}
	if allow, warning = LiveWatch(1, 0, []Soon{padded}, now); allow || !strings.Contains(warning, "News") {
		t.Fatalf("pad still wins outside 20s: allow=%v %q", allow, warning)
	}
	_, warning = LiveWatch(1, 0, []Soon{inside, {Title: "Game", ChannelID: 8, Start: now.Add(15 * time.Second), Pad: 0}}, now)
	if !strings.Contains(warning, "will miss 2 recordings") {
		t.Fatalf("%q", warning)
	}
}
