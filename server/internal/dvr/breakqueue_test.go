package dvr

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/store"
)

type queueRig struct {
	st      *store.Store
	q       *BreakQueue
	scanned chan string
	heard   chan string
	busy    atomic.Bool
}

func newQueueRig(t *testing.T, fail string) *queueRig {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &queueRig{st: st, scanned: make(chan string, 16), heard: make(chan string, 16)}
	r.q = newBreakQueue(st, func(_ context.Context, rec store.Recording) error {
		r.scanned <- rec.Title
		if rec.Title == fail {
			return errors.New("unreadable")
		}
		return nil
	}, func(ctx context.Context, rec store.Recording) error {
		r.heard <- rec.Title
		return st.SaveEpisodePrints(ctx, rec.ID, store.EpisodePrints{Head: []uint32{1}})
	}, r.busy.Load)
	r.q.root = "/data/recordings"
	r.q.poll = 10 * time.Millisecond
	return r
}

func (r *queueRig) add(t *testing.T, title, path, status string) int64 {
	t.Helper()
	id, err := r.st.CreateRecording(context.Background(), store.Recording{Title: title, Path: path, Status: status, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r *queueRig) run(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.q.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func (r *queueRig) next(t *testing.T) string {
	t.Helper()
	select {
	case title := <-r.scanned:
		return title
	case <-time.After(5 * time.Second):
		t.Fatal("nothing was scanned")
		return ""
	}
}

func (r *queueRig) none(t *testing.T, within time.Duration) {
	t.Helper()
	select {
	case title := <-r.scanned:
		t.Fatalf("scanned %s", title)
	case <-time.After(within):
	}
}

// At start the queue scans, oldest first, the finished recordings in the
// recordings folder that were never scanned and whose pass wants breaks.
func TestBreakQueueBackfillsUnscannedRecordings(t *testing.T) {
	ctx := context.Background()
	r := newQueueRig(t, "Broken")
	r.add(t, "Old News", "/data/recordings/old.ts", "complete")
	done := r.add(t, "Done", "/data/recordings/done.ts", "complete")
	if err := r.st.MarkBreaksScanned(ctx, done); err != nil {
		t.Fatal(err)
	}
	r.add(t, "Library Movie", "/media/movies/movie.ts", "complete")
	r.add(t, "Still Going", "/data/recordings/live.ts", "recording")
	if _, err := r.st.AddSeriesPass(ctx, store.Pass{Title: "Quiz Show", Commercials: false}); err != nil {
		t.Fatal(err)
	}
	r.add(t, "Quiz Show", "/data/recordings/quiz.ts", "complete")
	broken := r.add(t, "Broken", "/data/recordings/broken.ts", "complete")
	r.add(t, "New News", "/data/recordings/new.ts", "complete")
	r.run(t)
	for _, want := range []string{"Old News", "Broken", "New News"} {
		if got := r.next(t); got != want {
			t.Fatalf("scanned %s, want %s", got, want)
		}
	}
	r.none(t, 100*time.Millisecond)
	// A file that is gone is not read again on the next backfill.
	rec, err := r.st.Recording(ctx, broken)
	if err != nil || !rec.BreaksScanned {
		t.Fatalf("a failed scan was left unscanned: %+v %v", rec, err)
	}
}

// A file that is there but fails to scan (a share that was away, an ffmpeg
// that was stopped) gets two more tries before it is left alone.
func TestBreakQueueRetriesAFailedScan(t *testing.T) {
	ctx := context.Background()
	r := newQueueRig(t, "Flaky")
	path := filepath.Join(t.TempDir(), "flaky.ts")
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	r.q.root = ""
	id := r.add(t, "Flaky", path, "complete")
	r.run(t)
	r.next(t)
	if rec, _ := r.st.Recording(ctx, id); rec.BreaksScanned {
		t.Fatal("marked scanned after one failure")
	}
	r.q.Add(id)
	r.next(t)
	r.q.Add(id)
	r.next(t)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if rec, _ := r.st.Recording(ctx, id); rec.BreaksScanned {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("not left alone after three failures")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// A recording that ends while someone watches waits for a quiet moment, and
// runs anyway once the queue's patience is up.
func TestBreakQueueWaitsWhileSomeoneWatches(t *testing.T) {
	r := newQueueRig(t, "")
	r.busy.Store(true)
	r.run(t)
	r.q.Add(r.add(t, "Game", "/data/recordings/game.ts", "complete"))
	r.none(t, 150*time.Millisecond)
	r.busy.Store(false)
	if got := r.next(t); got != "Game" {
		t.Fatalf("scanned %s", got)
	}

	r2 := newQueueRig(t, "")
	r2.busy.Store(true)
	r2.q.patience = 200 * time.Millisecond
	r2.run(t)
	r2.q.Add(r2.add(t, "Movie", "/data/recordings/movie.ts", "complete"))
	if got := r2.next(t); got != "Movie" {
		t.Fatalf("scanned %s", got)
	}
}

// Every finished recording in the recordings folder is listened to for its
// intro, after its break scan, whether or not its pass wants breaks.
func TestBreakQueueListensToEveryRecording(t *testing.T) {
	ctx := context.Background()
	r := newQueueRig(t, "")
	done := r.add(t, "Done", "/data/recordings/done.ts", "complete")
	if err := r.st.MarkBreaksScanned(ctx, done); err != nil {
		t.Fatal(err)
	}
	if _, err := r.st.AddSeriesPass(ctx, store.Pass{Title: "Quiz Show", Commercials: false}); err != nil {
		t.Fatal(err)
	}
	quiz := r.add(t, "Quiz Show", "/data/recordings/quiz.ts", "complete")
	heard := r.add(t, "Heard", "/data/recordings/heard.ts", "complete")
	if err := r.st.MarkBreaksScanned(ctx, heard); err != nil {
		t.Fatal(err)
	}
	if err := r.st.SaveEpisodePrints(ctx, heard, store.EpisodePrints{}); err != nil {
		t.Fatal(err)
	}
	r.add(t, "Library Movie", "/media/movies/movie.ts", "complete")
	r.run(t)
	for _, want := range []string{"Done", "Quiz Show"} {
		select {
		case got := <-r.heard:
			if got != want {
				t.Fatalf("listened to %s, want %s", got, want)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s was not listened to", want)
		}
	}
	r.none(t, 100*time.Millisecond)
	select {
	case got := <-r.heard:
		t.Fatalf("listened to %s", got)
	default:
	}
	// Listening is no break scan: turning breaks on later still finds them.
	if rec, _ := r.st.Recording(ctx, quiz); rec.BreaksScanned || !rec.Listened {
		t.Fatalf("%+v", rec)
	}
}

// A scan that worked is not run again when listening failed, and a scan that
// failed does not keep the recording from being listened to.
func TestBreakQueueKeepsWhatWorked(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	path := filepath.Join(t.TempDir(), "show.ts")
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	var scans, listens atomic.Int32
	q := newBreakQueue(st, func(context.Context, store.Recording) error {
		scans.Add(1)
		return nil
	}, func(context.Context, store.Recording) error {
		listens.Add(1)
		return errors.New("share away")
	}, func() bool { return false })
	q.poll = 10 * time.Millisecond
	id, err := st.CreateRecording(ctx, store.Recording{Title: "Show", Path: path, Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		q.Run(runCtx)
		close(done)
	}()
	defer func() {
		cancel()
		<-done
	}()
	waitFor := func(what string, ok func() bool) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for !ok() {
			if time.Now().After(deadline) {
				t.Fatal(what)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	waitFor("not listened", func() bool { return listens.Load() == 1 })
	q.Add(id)
	waitFor("not listened again", func() bool { return listens.Load() == 2 })
	q.Add(id)
	waitFor("not given up", func() bool {
		rec, _ := st.Recording(ctx, id)
		return rec.Listened
	})
	if scans.Load() != 1 {
		t.Fatalf("scanned %d times", scans.Load())
	}
	if rec, _ := st.Recording(ctx, id); !rec.BreaksScanned {
		t.Fatal("a scan that worked was not marked")
	}
}
