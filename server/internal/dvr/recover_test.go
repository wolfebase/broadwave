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
