package dvr

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestRecoverKeepsThePieceAndResumesAShowStillOn(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(40*time.Minute + 20*time.Second)
	done, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 1, Title: "News", Path: "news.ts", Status: "recording", StartedAt: now.Add(-time.Hour), EndsAt: &past,
	})
	if err != nil {
		t.Fatal(err)
	}
	still, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 2, Title: "Game", Path: "game.ts", Status: "recording", StartedAt: now.Add(-20 * time.Minute), EndsAt: &future, PassID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 3, Title: "Saved", Path: "saved.ts", Status: "complete", StartedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	var resumed []store.Recording
	var lefts []time.Duration
	err = Recover(ctx, st, now, func(rec store.Recording, left time.Duration) error {
		resumed = append(resumed, rec)
		lefts = append(lefts, left)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resumed) != 1 || resumed[0].ID != still || resumed[0].PassID != 7 {
		t.Fatalf("resumed %+v", resumed)
	}
	if lefts[0] != 40*time.Minute+20*time.Second {
		t.Fatalf("time left %s, want the planned end with its padding", lefts[0])
	}
	for _, id := range []int64{done, still} {
		rec, err := st.Recording(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Status != "interrupted" || rec.Error != restartNote || !rec.Partial {
			t.Fatalf("recording %d status %q error %q partial %v", id, rec.Status, rec.Error, rec.Partial)
		}
	}
	rec, err := st.Recording(ctx, kept)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "complete" || rec.Partial {
		t.Fatalf("a finished recording was changed: %s partial %v", rec.Status, rec.Partial)
	}
}

// A clean stop already wrote "interrupted". The next start resumes it once;
// a start after that finds the second piece and leaves the show alone.
func TestRecoverResumesACleanStopOnce(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	ends := now.Add(30 * time.Minute)
	first, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 2, Title: "Game", Path: "a.ts", Status: "recording", StartedAt: now.Add(-time.Hour), EndsAt: &ends,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishRecording(ctx, first, "interrupted", restartNote); err != nil {
		t.Fatal(err)
	}
	calls := 0
	resume := func(rec store.Recording, left time.Duration) error {
		calls++
		_, err := st.CreateRecording(ctx, store.Recording{ChannelID: rec.ChannelID, Title: rec.Title, Path: "b.ts", Status: "recording", StartedAt: now, EndsAt: &ends})
		return err
	}
	if err := Recover(ctx, st, now, resume); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("resumed %d times", calls)
	}
	// The second piece is itself cut by another restart: it resumes, the first does not again.
	if err := Recover(ctx, st, now.Add(time.Minute), resume); err != nil {
		t.Fatal(err)
	}
	if calls != 2 {
		t.Fatalf("resumed %d times after the second restart, want 2", calls)
	}
	recs, _ := st.Recordings(ctx)
	if len(recs) != 3 {
		t.Fatalf("%d recordings, want 3 pieces", len(recs))
	}
}

type fakeRecorder struct {
	st      *store.Store
	minutes int
	meta    store.Recording
}

func (f *fakeRecorder) RecordMeta(ctx context.Context, minutes int, meta store.Recording) (store.Recording, error) {
	f.minutes, f.meta = minutes, meta
	now := time.Now()
	ends := now.Add(time.Duration(minutes) * time.Minute)
	meta.Status, meta.StartedAt, meta.EndsAt, meta.Path = "recording", now, &ends, "rest.ts"
	id, err := f.st.CreateRecording(ctx, meta)
	if err != nil {
		return store.Recording{}, err
	}
	return f.st.Recording(ctx, id)
}

func TestResumeCarriesThePassGameAndEpisode(t *testing.T) {
	st := openStore(t)
	ctx := context.Background()
	start := time.Now().Add(-40 * time.Minute)
	ends := start.Add(70 * time.Minute)
	prevID, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 4, Title: "Drama", Subtitle: "Pilot", ProgramID: "EP1", GameID: "", PassID: 9,
		Season: 3, Episode: 5, EpisodeLabel: "S3E5", Path: "a.ts", Status: "interrupted", StartedAt: start, EndsAt: &ends,
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, _ := st.Recording(ctx, prevID)
	rec := &fakeRecorder{st: st}
	got, err := Resume(ctx, st, rec, prev, 29*time.Minute+10*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if rec.minutes != 30 {
		t.Fatalf("asked for %d minutes, want the time left rounded up", rec.minutes)
	}
	// The rounded-up minutes would run past the show: the planned end goes along.
	if rec.meta.EndsAt == nil || !rec.meta.EndsAt.Equal(ends.Truncate(time.Second)) {
		t.Fatalf("resume ends %v, want the first piece's end %v", rec.meta.EndsAt, ends)
	}
	if rec.meta.PassID != 9 || rec.meta.ProgramID != "EP1" || rec.meta.Subtitle != "Pilot" {
		t.Fatalf("resume lost the listing: %+v", rec.meta)
	}
	saved, _ := st.Recording(ctx, got.ID)
	if !saved.Partial || saved.Season != 3 || saved.Episode != 5 || saved.EpisodeLabel != "S3E5" || saved.PassID != 9 {
		t.Fatalf("second piece %+v", saved)
	}
}

func TestRetryDue(t *testing.T) {
	now := time.Date(2026, 10, 1, 20, 30, 0, 0, time.UTC)
	air := store.Airing{ChannelID: 5, Title: "Movie", Start: now.Add(-30 * time.Minute), End: now.Add(90 * time.Minute)}
	pass := store.Pass{ID: 1, Title: "Movie", PadAfter: 2}
	failedAt := now.Add(-5 * time.Minute)
	failed := store.Recording{ChannelID: 5, Title: "Movie", Status: "failed", StartedAt: air.Start.Add(-time.Minute), EndedAt: &failedAt}
	if !RetryDue([]store.Recording{failed}, pass, air, now) {
		t.Fatal("a recording that failed partway was not tried again")
	}
	if RetryDue(nil, pass, air, now) {
		t.Fatal("an airing never attempted is a start, not a retry")
	}
	stopped := failed
	stopped.Status = "complete"
	if RetryDue([]store.Recording{stopped}, pass, air, now) {
		t.Fatal("a recording the viewer stopped was tried again")
	}
	justNow := now.Add(-20 * time.Second)
	fresh := failed
	fresh.EndedAt = &justNow
	if RetryDue([]store.Recording{fresh}, pass, air, now) {
		t.Fatal("tried again before the wait")
	}
	var many []store.Recording
	for i := 0; i <= maxRetries; i++ {
		many = append(many, failed)
	}
	if RetryDue(many, pass, air, now) {
		t.Fatal("tried again past the cap")
	}
	over := air
	over.End = now.Add(-3 * time.Minute)
	if RetryDue([]store.Recording{failed}, pass, over, now) {
		t.Fatal("tried again after the show and its padding ended")
	}
	earlier := failed
	earlier.StartedAt = air.Start.Add(-3 * time.Hour)
	if RetryDue([]store.Recording{earlier}, pass, air, now) {
		t.Fatal("an earlier episode's failure counted for this airing")
	}
}

// A partial piece is not the episode: the next airing still records, and a
// keep rule neither counts nor removes it.
func TestPartialPiecesAreNotTheEpisode(t *testing.T) {
	now := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	pass := store.Pass{ID: 1, Title: "Drama", KeepMode: "last", KeepCount: 1}
	air := store.Airing{ID: 9, ChannelID: 2, Title: "Drama", ProgramID: "EP7", Start: now, End: now.Add(time.Hour)}
	piece := store.Recording{ID: 3, ChannelID: 2, Title: "Drama", ProgramID: "EP7", Status: "complete", Partial: true, PassID: 1, StartedAt: now.Add(-48 * time.Hour)}
	cut := store.Recording{ID: 4, ChannelID: 2, Title: "Drama", ProgramID: "EP7", Status: "interrupted", PassID: 1, StartedAt: now.Add(-49 * time.Hour)}
	items := ApplyLibrary([]Planned{{PassID: 1, Airing: air}}, []store.Pass{pass}, []store.Recording{piece, cut}, map[string]bool{}, map[string]bool{})
	if items[0].Skipped {
		t.Fatalf("skipped as %q with only partial pieces", items[0].Reason)
	}
	whole := store.Recording{ID: 5, ChannelID: 2, Title: "Drama", ProgramID: "EP6", Status: "complete", PassID: 1, StartedAt: now.Add(-24 * time.Hour)}
	if victims := KeepVictims(pass, []store.Recording{piece, cut, whole}); len(victims) != 0 {
		t.Fatalf("keep rule removed %v; partial pieces must not push out the whole episode", victims)
	}
}

func TestPlanCountsPaddingAndSportsTime(t *testing.T) {
	at := time.Date(2026, 10, 1, 20, 0, 0, 0, time.UTC)
	first := store.Airing{ID: 1, ChannelID: 1, Title: "Early", Start: at, End: at.Add(time.Hour)}
	second := store.Airing{ID: 2, ChannelID: 2, Title: "Late", Start: at.Add(time.Hour), End: at.Add(2 * time.Hour)}
	passes := []store.Pass{{ID: 1, Title: "Early", Priority: 2}, {ID: 2, Title: "Late", Priority: 1}}
	items := Plan(passes, []store.Airing{first, second}, 1, at.Add(-time.Hour), at.Add(3*time.Hour))
	for _, item := range items {
		if item.Conflict {
			t.Fatalf("back-to-back shows without padding conflict: %+v", item)
		}
	}
	passes[0].PadAfter = 5
	items = Plan(passes, []store.Airing{first, second}, 1, at.Add(-time.Hour), at.Add(3*time.Hour))
	if !items[1].Conflict || items[0].Conflict {
		t.Fatalf("five minutes of padding over the next show on the only tuner is a conflict: %+v", items)
	}
	passes[0].PadAfter = 0
	game := first
	game.Title, game.Category = "Early", "Sports event, Football"
	items = Plan(passes, []store.Airing{game, second}, 1, at.Add(-time.Hour), at.Add(3*time.Hour))
	if !items[1].Conflict {
		t.Fatal("a sports listing's extra hour did not hold the tuner")
	}
	game.GameID = "401"
	items = Plan(passes, []store.Airing{game, second}, 1, at.Add(-time.Hour), at.Add(3*time.Hour))
	if items[1].Conflict {
		t.Fatal("a matched game is followed live; its listing alone does not conflict")
	}
	// A matched game already running long holds its tuner as long as its recording.
	long := at.Add(80 * time.Minute)
	running := []store.Recording{{ChannelID: 1, Title: "Early", Status: "recording", StartedAt: at, EndsAt: &long}}
	items = PlanLibrary(passes, []store.Airing{game, second}, 1, at.Add(-time.Hour), at.Add(3*time.Hour), running, nil, nil)
	if !items[1].Conflict {
		t.Fatal("a game running into the next show did not hold the tuner")
	}
}

func TestLiveTunersSkipQuietAndRetiredDevices(t *testing.T) {
	now := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	fresh := now.Add(-4 * time.Minute).Format(time.RFC3339)
	stale := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	dev := func(id string, tuners int, seen string, retired bool) store.Device {
		d := store.Device{LastSeen: seen, Retired: retired}
		d.DeviceID, d.TunerCount = id, tuners
		return d
	}
	cases := []struct {
		name    string
		devices []store.Device
		want    int
	}{
		{"both answer", []store.Device{dev("FLEX", 4, fresh, false), dev("DUO", 2, fresh, false)}, 6},
		{"one went quiet two weeks ago", []store.Device{dev("FLEX", 4, fresh, false), dev("DUO", 2, stale, false)}, 4},
		{"one retired", []store.Device{dev("FLEX", 4, fresh, false), dev("DUO", 2, fresh, true)}, 4},
		{"all quiet keeps the ones not retired", []store.Device{dev("FLEX", 4, stale, false), dev("DUO", 2, stale, true)}, 4},
		{"playlists only", []store.Device{dev("src-1", 0, fresh, false)}, 2},
		{"nothing yet", nil, 2},
	}
	for _, c := range cases {
		if got := LiveTuners(c.devices, now); got != c.want {
			t.Errorf("%s: %d, want %d", c.name, got, c.want)
		}
	}
}

// Next week's episode of the same title is not the resume of this one.
func TestResumedLaterIgnoresALaterEpisode(t *testing.T) {
	now := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	ends := now.Add(time.Hour)
	cut := store.Recording{ID: 1, ChannelID: 2, Title: "Drama", StartedAt: now, EndsAt: &ends}
	nextWeek := store.Recording{ID: 2, ChannelID: 2, Title: "Drama", StartedAt: now.Add(7 * 24 * time.Hour)}
	if resumedLater([]store.Recording{cut, nextWeek}, cut) {
		t.Fatal("a later episode counted as the resume")
	}
	rest := store.Recording{ID: 3, ChannelID: 2, Title: "Drama", StartedAt: now.Add(30 * time.Minute)}
	if !resumedLater([]store.Recording{cut, rest}, cut) {
		t.Fatal("the second piece was not seen")
	}
}
