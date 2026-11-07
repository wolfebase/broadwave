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

func TestASimulcastIsRecordedOnce(t *testing.T) {
	start := time.Date(2026, 10, 4, 18, 0, 0, 0, time.UTC)
	airings := []store.Airing{
		{ID: 1, ChannelID: 1, Title: "Football", Start: start, End: start.Add(3 * time.Hour), Simulcast: 2},
		{ID: 2, ChannelID: 2, Title: "Football", Start: start, End: start.Add(3 * time.Hour)},
	}
	got := Plan([]store.Pass{{ID: 1, Title: "Football"}}, airings, 2, start.Add(-time.Minute), start.Add(4*time.Hour))
	if len(got) != 1 || got[0].Airing.ID != 2 {
		t.Fatalf("series pass planned %+v, want only the airing on channel 2", got)
	}
	got = Plan([]store.Pass{{ID: 1, Title: "Football", ChannelID: 1}}, airings, 2, start.Add(-time.Minute), start.Add(4*time.Hour))
	if len(got) != 1 || got[0].Airing.ID != 1 {
		t.Fatalf("a pass pinned to channel 1 planned %+v, want its own airing", got)
	}
}

func TestAPassRecordsOnlyOnItsDays(t *testing.T) {
	// Times are local; build them in time.Local so the weekday is the viewer's.
	at := func(day, hour int) time.Time {
		// 2026-10-04 is a Sunday.
		return time.Date(2026, 10, 4+day, hour, 0, 0, 0, time.Local)
	}
	weeknights := store.Pass{ID: 1, Title: "Late Show", Days: []int{1, 2, 3, 4, 5}, TimeStart: "22:00", TimeEnd: "02:00"}
	cases := []struct {
		name  string
		start time.Time
		want  bool
	}{
		{"Monday night", at(1, 22), true},
		{"after midnight on Friday night", at(6, 1), true},
		{"after midnight on Sunday night", at(1, 1), false},
		{"Saturday night", at(6, 23), false},
		{"Monday afternoon", at(1, 15), false},
	}
	for _, tc := range cases {
		if got := inWindow(weeknights, tc.start); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.name, got, tc.want)
		}
	}
	sundays := store.Pass{ID: 2, Title: "Brunch", Days: []int{0}}
	if !inWindow(sundays, at(0, 9)) || inWindow(sundays, at(1, 9)) {
		t.Error("a days-only pass records on its day at any hour and no other day")
	}
	if !inWindow(store.Pass{ID: 3, Title: "Any"}, at(3, 3)) {
		t.Error("a pass with no days records every day")
	}
}

func tuneAiring(id, channel int64, tune string, start time.Time) store.Airing {
	return store.Airing{ID: id, ChannelID: channel, Tune: tune, Title: "Show", Start: start, End: start.Add(time.Hour)}
}

// Two frequencies carry four channels, so two tuners record all four. The plan
// used to count channels, skip two, and start them a tick after the others.
func TestChannelsOnOneFrequencyShareATuner(t *testing.T) {
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	passes := []store.Pass{{ID: 1, Title: "Show", Kind: "series"}}
	airings := []store.Airing{
		tuneAiring(1, 41, "box:593", start),
		tuneAiring(2, 42, "box:593", start),
		tuneAiring(3, 51, "box:605", start),
		tuneAiring(4, 52, "box:605", start),
	}
	items := Plan(passes, airings, 2, start.Add(-time.Hour), start.Add(2*time.Hour))
	if len(items) != 4 {
		t.Fatalf("items %+v", items)
	}
	for _, item := range items {
		if item.Skipped || item.Conflict {
			t.Fatalf("a false conflict on channel %d: %+v", item.Airing.ChannelID, item)
		}
	}
}

// A third frequency is one too many. Everything on it goes together, and the
// tune that goes is the one whose best show ranks lowest. Dropping the lowest
// show on a frequency that stays tuned would free nothing.
func TestAFrequencyGoesAsAWhole(t *testing.T) {
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	passes := []store.Pass{
		{ID: 1, Title: "Top", Kind: "series", Priority: 9},
		{ID: 2, Title: "Mid", Kind: "series", Priority: 5},
		{ID: 3, Title: "Low", Kind: "series", Priority: 3},
		{ID: 4, Title: "Least", Kind: "series", Priority: 1},
	}
	show := func(id, channel int64, tune, title string) store.Airing {
		return store.Airing{ID: id, ChannelID: channel, Tune: tune, Title: title, Start: start, End: start.Add(time.Hour)}
	}
	airings := []store.Airing{
		show(1, 41, "box:593", "Top"),
		show(2, 42, "box:593", "Least"),
		show(3, 51, "box:605", "Mid"),
		show(4, 61, "box:617", "Low"),
	}
	items := Plan(passes, airings, 2, start.Add(-time.Hour), start.Add(2*time.Hour))
	for _, item := range items {
		wantSkip := item.Airing.Title == "Low"
		if item.Skipped != wantSkip || item.Conflict != wantSkip {
			t.Fatalf("%s skipped=%v conflict=%v, want only Low to go", item.Airing.Title, item.Skipped, item.Conflict)
		}
		if wantSkip && item.Reason == "" {
			t.Fatalf("a conflict gives no reason: %+v", item)
		}
	}
}

// A later airing is offered when it fits on a frequency already tuned.
func TestASuggestionCountsFrequencies(t *testing.T) {
	start := time.Now().Add(time.Hour).Truncate(time.Minute)
	spans := []span{
		{start: start, end: start.Add(time.Hour), tune: "box:593"},
		{start: start, end: start.Add(time.Hour), tune: "box:593"},
		{start: start, end: start.Add(time.Hour), tune: "box:605"},
	}
	if overflows(spans, 2) {
		t.Fatal("two frequencies fit on two tuners")
	}
	spans = append(spans, span{start: start, end: start.Add(time.Hour), tune: "box:617"})
	if !overflows(spans, 2) {
		t.Fatal("three frequencies do not fit on two tuners")
	}
}

// The warning before a live watch counts tuners the same way.
func TestALiveWarningCountsFrequencies(t *testing.T) {
	now := time.Now()
	soon := now.Add(time.Minute)
	upcoming := []Soon{
		{Title: "A", ChannelID: 41, Tune: "box:593", Start: soon},
		{Title: "B", ChannelID: 42, Tune: "box:593", Start: soon},
	}
	if allow, warning := LiveWatch(2, 0, upcoming, now); !allow {
		t.Fatalf("one frequency needs one tuner, and one is left: %q", warning)
	}
}

func underwayFixture(now time.Time) ([]store.Pass, []store.Airing) {
	start := now.Add(-21 * time.Minute)
	passes := []store.Pass{{ID: 1, Title: "Show", Kind: "series"}}
	return passes, []store.Airing{
		{ID: 1, ChannelID: 4, Title: "Show", Start: start, End: start.Add(30 * time.Minute)},
		{ID: 2, ChannelID: 4, Title: "Show", Start: start.Add(30 * time.Minute), End: start.Add(time.Hour)},
	}
}

func planUnderway(now time.Time, recs []store.Recording, air OnAir) []Planned {
	passes, airings := underwayFixture(now)
	air.Now = now
	return PlanOnAir(passes, airings, 2, now, now.Add(2*time.Hour), recs, nil, nil, air)
}

// A rule made while its airing is on: that airing never starts, so the plan
// doesn't say it will. The next one still does.
func TestAnAiringAlreadyOnIsNotPromised(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	items := planUnderway(now, nil, OnAir{})
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	if !items[0].Skipped || items[0].Conflict || items[0].Reason != "Already on" {
		t.Fatalf("under way: %+v", items[0])
	}
	if items[1].Skipped {
		t.Fatalf("the next airing: %+v", items[1])
	}
}

func TestAnAiringOnThatWillRecordStaysPromised(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	passes, airings := underwayFixture(now)
	on := airings[0]
	for name, c := range map[string]struct {
		recs []store.Recording
		air  OnAir
	}{
		"recording":              {recs: []store.Recording{{ChannelID: 4, Title: "Show", Status: "recording", StartedAt: on.Start}}},
		"failed with tries left": {recs: []store.Recording{{ChannelID: 4, Title: "Show", Status: "failed", StartedAt: on.Start, EndedAt: &now}}},
		"waiting for a tuner":    {air: OnAir{Waits: []store.RecordingWait{{ChannelID: 4, Start: on.Start}}}},
		"held by the buffer":     {air: OnAir{Held: func(int64) time.Time { return on.Start.Add(-time.Minute) }}},
	} {
		items := planUnderway(now, c.recs, c.air)
		if items[0].Skipped {
			t.Errorf("%s: %+v", name, items[0])
		}
	}
	// Inside the start window Tick still starts it.
	inWindow := on
	inWindow.Start = now.Add(-time.Minute)
	got := PlanOnAir(passes, []store.Airing{inWindow}, 2, now, now.Add(time.Hour), nil, nil, nil, OnAir{Now: now})
	if len(got) != 1 || got[0].Skipped {
		t.Fatalf("a minute in: %+v", got)
	}
	// A buffer that starts after the show did cannot make it whole.
	late := planUnderway(now, nil, OnAir{Held: func(int64) time.Time { return on.Start.Add(time.Minute) }})
	if !late[0].Skipped {
		t.Fatalf("buffer after the start: %+v", late[0])
	}
}

// An airing nothing will start must not push a real recording off the tuners.
func TestAnAiringAlreadyOnTakesNoTuner(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	passes := []store.Pass{{ID: 1, Title: "On", Kind: "series", Priority: 5}, {ID: 2, Title: "Next", Kind: "series", Priority: 1}}
	start := now.Add(-20 * time.Minute)
	airings := []store.Airing{
		{ID: 1, ChannelID: 4, Title: "On", Start: start, End: now.Add(time.Hour)},
		{ID: 2, ChannelID: 5, Title: "Next", Start: now.Add(time.Minute), End: now.Add(time.Hour)},
	}
	items := PlanOnAir(passes, airings, 1, now, now.Add(2*time.Hour), nil, nil, nil, OnAir{Now: now})
	if !items[0].Skipped || items[0].Reason != "Already on" {
		t.Fatalf("on: %+v", items[0])
	}
	if items[1].Skipped {
		t.Fatalf("next lost its tuner to a show that won't record: %+v", items[1])
	}
}
