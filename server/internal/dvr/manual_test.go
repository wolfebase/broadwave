package dvr

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

// inZone runs the test with the server's clock in name.
func inZone(t *testing.T, name string) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Skipf("no zone data: %v", err)
	}
	prev := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = prev })
	return loc
}

func TestManualOnceRecordsWithoutListings(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	start := time.Date(2026, 10, 21, 22, 0, 0, 0, loc)
	pass := store.Pass{ID: 7, Kind: "manual", ChannelID: 3, TimeStart: "22:00", Duration: 90, AiringStart: start}
	got := Plan([]store.Pass{pass}, nil, 2, start.Add(-time.Hour), start.Add(24*time.Hour))
	if len(got) != 1 {
		t.Fatalf("want one slot, got %+v", got)
	}
	item := got[0]
	if !item.Manual || item.PassID != 7 || item.Airing.ChannelID != 3 || item.Airing.ID >= 0 {
		t.Fatalf("slot: %+v", item)
	}
	if !item.Airing.Start.Equal(start) || !item.Airing.End.Equal(start.Add(90*time.Minute)) {
		t.Fatalf("times: %v – %v", item.Airing.Start, item.Airing.End)
	}
	// A slot already on stays in the plan, so it keeps its tuner and can retry.
	if late := Plan([]store.Pass{pass}, nil, 2, start.Add(80*time.Minute), start.Add(2*time.Hour)); len(late) != 1 {
		t.Fatalf("a slot still on drops out: %+v", late)
	}
	if after := Plan([]store.Pass{pass}, nil, 2, start.Add(90*time.Minute), start.Add(3*time.Hour)); len(after) != 0 {
		t.Fatalf("an ended slot is still planned: %+v", after)
	}
}

func TestManualRepeatKeepsLocalTimeAcrossDST(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	// Every day at 22:00 over the first Sunday of November 2026, when Chicago falls back.
	pass := store.Pass{ID: 1, Kind: "manual", ChannelID: 2, TimeStart: "22:00", Duration: 30, Days: []int{0, 1, 2, 3, 4, 5, 6}}
	from := time.Date(2026, 10, 30, 12, 0, 0, 0, loc)
	to := time.Date(2026, 11, 3, 12, 0, 0, 0, loc)
	starts := ManualStarts(pass, from, to)
	if len(starts) != 4 {
		t.Fatalf("want 4 nights, got %v", starts)
	}
	offsets := map[int]bool{}
	for _, s := range starts {
		local := s.In(loc)
		if local.Hour() != 22 || local.Minute() != 0 {
			t.Fatalf("slot moved off 22:00: %v", local)
		}
		_, off := local.Zone()
		offsets[off] = true
	}
	if len(offsets) != 2 {
		t.Fatalf("the plan should cross the change: %v", starts)
	}
	if got := starts[2].Sub(starts[1]); got != 25*time.Hour {
		t.Fatalf("the night of the change is 25 hours on: %v", got)
	}
}

func TestManualSpringGapStartsAnHourLater(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	// 2:30 does not exist on 2027-03-14 in Chicago.
	pass := store.Pass{ID: 1, Kind: "manual", ChannelID: 2, TimeStart: "02:30", Duration: 30, Days: []int{0}}
	starts := ManualStarts(pass, time.Date(2027, 3, 13, 12, 0, 0, 0, loc), time.Date(2027, 3, 15, 12, 0, 0, 0, loc))
	if len(starts) != 1 || starts[0].In(loc).Hour() != 3 || starts[0].In(loc).Minute() != 30 {
		t.Fatalf("want 03:30 on the change day, got %v", starts)
	}
}

func TestManualWeekdaysAndOvernight(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	// Tuesdays at 23:30 for 2 hours: runs past midnight, still Tuesday's slot.
	pass := store.Pass{ID: 1, Kind: "manual", ChannelID: 2, TimeStart: "23:30", Duration: 120, Days: []int{2}}
	from := time.Date(2026, 10, 18, 0, 0, 0, 0, loc) // Sunday
	to := from.Add(14 * 24 * time.Hour)
	starts := ManualStarts(pass, from, to)
	if len(starts) != 2 {
		t.Fatalf("want two Tuesdays, got %v", starts)
	}
	for _, s := range starts {
		if s.In(loc).Weekday() != time.Tuesday {
			t.Fatalf("not a Tuesday: %v", s)
		}
	}
	// Wednesday 00:30, the slot from Tuesday night is still on.
	wed := time.Date(2026, 10, 21, 0, 30, 0, 0, loc)
	if got := ManualStarts(pass, wed, wed.Add(time.Hour)); len(got) != 1 {
		t.Fatalf("the slot running past midnight is missing: %v", got)
	}
}

func TestManualTakesListingDetails(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	start := time.Date(2026, 10, 21, 22, 0, 0, 0, loc)
	airings := []store.Airing{
		{ID: 10, ChannelID: 2, Title: "Late Movie", Description: "A film.", ProgramID: "MV1", Start: start.Add(-30 * time.Minute), End: start.Add(15 * time.Minute)},
		{ID: 11, ChannelID: 2, Title: "Night News", Subtitle: "Tuesday", Description: "The news.", ProgramID: "EP2", Category: "News", Start: start.Add(15 * time.Minute), End: start.Add(75 * time.Minute)},
		{ID: 12, ChannelID: 4, Title: "Elsewhere", Start: start, End: start.Add(time.Hour)},
	}
	pass := store.Pass{ID: 3, Kind: "manual", ChannelID: 2, TimeStart: "22:00", Duration: 60, AiringStart: start}
	got := Plan([]store.Pass{pass}, airings, 2, start.Add(-time.Hour), start.Add(2*time.Hour))
	if len(got) != 1 {
		t.Fatalf("manual only records its own slot, got %+v", got)
	}
	air := got[0].Airing
	if air.Title != "Night News" || air.Description != "The news." || air.ProgramID != "EP2" {
		t.Fatalf("want the listing that fills most of the slot: %+v", air)
	}
	if !air.Start.Equal(start) || !air.End.Equal(start.Add(time.Hour)) {
		t.Fatalf("the slot keeps its own time: %+v", air)
	}
	pass.Title = "My show"
	if got := Plan([]store.Pass{pass}, airings, 2, start.Add(-time.Hour), start.Add(2*time.Hour)); got[0].Airing.Title != "My show" || got[0].Airing.Description != "The news." {
		t.Fatalf("a title the viewer gave wins: %+v", got[0].Airing)
	}
}

func TestManualSharesChannelAndConflictsAcross(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	start := time.Date(2026, 10, 21, 22, 0, 0, 0, loc)
	airings := []store.Airing{
		{ID: 1, ChannelID: 2, Title: "Night News", Start: start, End: start.Add(30 * time.Minute)},
		{ID: 2, ChannelID: 5, Title: "Drama", Start: start, End: start.Add(time.Hour)},
	}
	manual := store.Pass{ID: 9, Kind: "manual", ChannelID: 2, TimeStart: "22:00", Duration: 60, AiringStart: start, Priority: 1}
	series := store.Pass{ID: 1, Kind: "series", Title: "Night News", MatchKind: "title"}
	// The same channel takes one tuner.
	got := Plan([]store.Pass{manual, series}, airings, 1, start.Add(-time.Minute), start.Add(2*time.Hour))
	if len(got) != 2 {
		t.Fatalf("want the series airing and the manual slot: %+v", got)
	}
	for _, item := range got {
		if item.Skipped {
			t.Fatalf("one channel fits one tuner: %+v", got)
		}
	}
	// Another channel at the same time on one tuner: priority decides.
	drama := store.Pass{ID: 2, Kind: "series", Title: "Drama", MatchKind: "title", Priority: 5}
	got = Plan([]store.Pass{drama, manual}, airings, 1, start.Add(-time.Minute), start.Add(2*time.Hour))
	var manualItem, dramaItem Planned
	for _, item := range got {
		if item.Manual {
			manualItem = item
		} else {
			dramaItem = item
		}
	}
	if !manualItem.Skipped || !manualItem.Conflict || dramaItem.Skipped {
		t.Fatalf("the higher pass keeps the tuner: %+v", got)
	}
	got = AttachSuggestions(got, []store.Pass{drama, manual}, airings, 1, start.Add(-time.Minute), start.Add(2*time.Hour), nil)
	for _, item := range got {
		if item.Manual && item.Suggestion != nil {
			t.Fatalf("a manual slot has no other airing to move to: %+v", item)
		}
	}
}

func TestManualLibraryRules(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	start := time.Date(2026, 10, 21, 22, 0, 0, 0, loc)
	airings := []store.Airing{{ID: 1, ChannelID: 2, Title: "Night News", ProgramID: "EP2", Start: start, End: start.Add(time.Hour)}}
	pass := store.Pass{ID: 4, Kind: "manual", ChannelID: 2, TimeStart: "22:00", Duration: 60, Days: []int{int(start.Weekday())}}
	done := store.Recording{ID: 1, ChannelID: 2, Title: "Night News", ProgramID: "EP2", Status: "complete", PassID: 4, StartedAt: start.Add(-7 * 24 * time.Hour)}
	from, to := start.Add(-time.Minute), start.Add(2*time.Hour)

	got := PlanLibrary([]store.Pass{pass}, airings, 2, from, to, []store.Recording{done}, nil, nil)
	if len(got) != 1 || got[0].Skipped {
		t.Fatalf("a manual slot records the time even if the episode was recorded: %+v", got)
	}

	pass.LimitCount = 1
	got = PlanLibrary([]store.Pass{pass}, airings, 2, from, to, []store.Recording{done}, nil, nil)
	if !got[0].Skipped || got[0].Reason != "Limit reached" {
		t.Fatalf("the limit applies: %+v", got)
	}

	pass.LimitCount = 0
	key, starts := SkipParts(got[0].Airing)
	if key != ManualSkipKey(4) {
		t.Fatalf("skip key: %q", key)
	}
	got = PlanLibrary([]store.Pass{pass}, airings, 2, from, to, nil, nil, map[string]bool{key + "|" + starts: true})
	if !got[0].Skipped || got[0].Reason != "Skipped once" {
		t.Fatalf("skip once: %+v", got)
	}
	// Skipping the manual slot leaves a series pass on the same episode alone.
	series := store.Pass{ID: 1, Kind: "series", Title: "Night News", MatchKind: "title"}
	got = PlanLibrary([]store.Pass{pass, series}, airings, 2, from, to, nil, nil, map[string]bool{key + "|" + starts: true})
	for _, item := range got {
		if !item.Manual && item.Skipped {
			t.Fatalf("the series airing was skipped too: %+v", got)
		}
	}

	if !SameShow(pass, done) || SameShow(pass, store.Recording{ChannelID: 2, Title: "Night News"}) {
		t.Fatal("keep rules cover the pass's own recordings only")
	}
}

func TestManualRetryAndAlreadyByPass(t *testing.T) {
	loc := inZone(t, "America/Chicago")
	start := time.Date(2026, 10, 21, 22, 0, 0, 0, loc)
	pass := store.Pass{ID: 4, Kind: "manual", ChannelID: 2, TimeStart: "22:00", Duration: 60, AiringStart: start}
	air := Plan([]store.Pass{pass}, nil, 2, start.Add(-time.Minute), start.Add(time.Hour))[0].Airing
	ended := start.Add(10 * time.Minute)
	// The guide renamed the slot after it started; the pass still names the recording.
	failed := store.Recording{ID: 1, ChannelID: 2, Title: "2.1 Station", Status: "failed", PassID: 4, StartedAt: start, EndedAt: &ended}
	if !RetryDue([]store.Recording{failed}, pass, air, ended.Add(2*time.Minute)) {
		t.Fatal("a failed manual slot retries")
	}
	other := failed
	other.PassID = 99
	if RetryDue([]store.Recording{other}, pass, air, ended.Add(2*time.Minute)) {
		t.Fatal("another pass's recording is not this slot's")
	}
	running := store.Recording{ID: 2, ChannelID: 2, Title: "Night News", Status: "recording", PassID: 4, StartedAt: start}
	if !already([]store.Recording{running}, air) {
		t.Fatal("the slot is already recording")
	}
	listing := store.Airing{ID: 5, ChannelID: 2, Title: "Night News", Start: start, End: start.Add(time.Hour)}
	if !already([]store.Recording{running}, listing) {
		t.Fatal("a listing on the same channel is the same feed")
	}
	if already([]store.Recording{{ID: 3, ChannelID: 2, Title: "Night News", Status: "recording", PassID: 1, StartedAt: start}}, air) {
		t.Fatal("a series recording of the listing is not the manual slot")
	}
	if ok, minutes := StartDecision(pass, air, start.Add(-time.Minute)); !ok || minutes != 62 {
		t.Fatalf("the slot starts on time for the hour and the minute before: %v %d", ok, minutes)
	}
}
