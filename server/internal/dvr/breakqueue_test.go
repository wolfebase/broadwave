package dvr

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"broadwave/internal/breaks"
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

// A library series with two or more episodes is listened to, one file at a
// time, and the intro is stored on the recording. One episode is left alone.
// Nothing is written next to the library files.
func TestLibrarySeriesIntrosAreFound(t *testing.T) {
	ctx := context.Background()
	r := newQueueRig(t, "")
	lib := t.TempDir()
	recordings := t.TempDir()
	r.q.root = recordings
	theme := prints(1, 30, nil, 0)
	heads := map[string][]uint32{}
	put := func(title, path string, season, episode int, at float64, day int) int64 {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ts"), 0o644); err != nil {
			t.Fatal(err)
		}
		heads[path] = prints(int64(season*10+episode), 1200, theme, at)
		id, err := r.st.CreateRecording(ctx, store.Recording{
			Title: title, Path: path, Status: "complete",
			Season: season, Episode: episode,
			StartedAt: time.Now().AddDate(0, 0, day),
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := r.st.SetDuration(ctx, id, 1800); err != nil {
			t.Fatal(err)
		}
		return id
	}
	one := put("Harbor Watch", filepath.Join(lib, "Harbor", "S01E01.ts"), 1, 1, 90, -5)
	two := put("Harbor Watch", filepath.Join(lib, "Harbor", "S01E02.ts"), 1, 2, 200, -4)
	three := put("Harbor Watch", filepath.Join(lib, "Harbor", "S01E03.ts"), 1, 3, 40, -3)
	solo := put("Only Once", filepath.Join(lib, "Only", "S01E01.ts"), 1, 1, 90, -2)
	rerunA := put("Rerun", filepath.Join(lib, "Rerun", "S01E01a.ts"), 1, 1, 90, -2)
	rerunB := put("Rerun", filepath.Join(lib, "Rerun", "S01E01b.ts"), 1, 1, 200, -1)
	put("Split Show", filepath.Join(recordings, "S01E01.ts"), 1, 1, 90, -1)
	splitLib := put("Split Show", filepath.Join(lib, "Split", "S01E02.ts"), 1, 2, 200, 0)

	old := listenFile
	listenFile = func(_ context.Context, _, path string, _ float64) (breaks.Sound, error) {
		return breaks.Sound{Head: heads[path]}, nil
	}
	t.Cleanup(func() { listenFile = old })
	var mu sync.Mutex
	var scanned []string
	r.q.scan = func(_ context.Context, rec store.Recording) error {
		mu.Lock()
		scanned = append(scanned, rec.Path)
		mu.Unlock()
		return nil
	}
	r.q.listen = func(ctx context.Context, rec store.Recording) error {
		return FindEpisodeEnds(ctx, r.st, "", rec)
	}
	before := dirListing(t, lib)
	r.run(t)

	wantIntro := map[int64]float64{one: 90, two: 200, three: 40}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		ready := true
		for id, start := range wantIntro {
			got, _ := r.st.Recording(ctx, id)
			if math.Abs(got.IntroStart-start) > 1 || math.Abs(got.IntroEnd-start-30) > 1 {
				ready = false
			}
		}
		split, _ := r.st.Recording(ctx, splitLib)
		if !split.Listened {
			ready = false
		}
		if ready {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	for id, start := range wantIntro {
		got, _ := r.st.Recording(ctx, id)
		if math.Abs(got.IntroStart-start) > 1 || math.Abs(got.IntroEnd-start-30) > 1 {
			t.Fatalf("recording %d: intro %v-%v, want %v", id, got.IntroStart, got.IntroEnd, start)
		}
	}
	for _, id := range []int64{solo, rerunA, rerunB} {
		got, _ := r.st.Recording(ctx, id)
		if got.Listened || got.IntroEnd != 0 {
			t.Fatalf("episode that stands alone was listened to: %+v", got)
		}
	}
	if got := dirListing(t, lib); !slices.Equal(before, got) {
		t.Fatalf("library folder changed\nbefore %v\nafter  %v", before, got)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, path := range scanned {
		if r.q.isLibraryPath(path) {
			t.Fatalf("scanned a library file: %s", path)
		}
	}
}

// A library file waits while someone is watching, including past the moment
// a recording in the recordings folder would run anyway.
func TestLibraryFileWaitsWhileSomeoneWatches(t *testing.T) {
	r := newQueueRig(t, "")
	r.busy.Store(true)
	r.q.patience = 30 * time.Millisecond
	r.addEpisode(t, "Show", "/media/lib/S01E01.ts", 1, 1)
	r.addEpisode(t, "Show", "/media/lib/S01E02.ts", 1, 2)
	r.run(t)
	select {
	case title := <-r.heard:
		t.Fatalf("heard %s while someone was watching", title)
	case title := <-r.scanned:
		t.Fatalf("scanned %s", title)
	case <-time.After(200 * time.Millisecond):
	}
	r.busy.Store(false)
	select {
	case title := <-r.heard:
		if title != "Show" {
			t.Fatalf("heard %s", title)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("not listened to once the screens were quiet")
	}
}

// Six library files an hour, and never two at once. The seventh waits out the hour.
func TestLibraryListenCapIsSixAnHour(t *testing.T) {
	r := newQueueRig(t, "")
	for episode := 1; episode <= 7; episode++ {
		r.addEpisode(t, "Marathon", filepath.Join("/media/lib", fmt.Sprintf("e%d.ts", episode)), 1, episode)
	}
	start := time.Date(2026, 10, 9, 12, 0, 0, 0, time.UTC)
	var wall atomic.Int64
	wall.Store(start.UnixNano())
	r.q.now = func() time.Time { return time.Unix(0, wall.Load()).UTC() }
	var inflight, maxIn atomic.Int32
	r.q.listen = func(ctx context.Context, rec store.Recording) error {
		n := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			m := maxIn.Load()
			if n <= m || maxIn.CompareAndSwap(m, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond)
		if err := r.st.SaveEpisodePrints(ctx, rec.ID, store.EpisodePrints{Head: []uint32{1}}); err != nil {
			return err
		}
		r.heard <- rec.Title
		return nil
	}
	r.run(t)
	for range 6 {
		select {
		case <-r.heard:
		case <-time.After(5 * time.Second):
			t.Fatal("stopped before six files")
		}
	}
	select {
	case <-r.heard:
		t.Fatal("a seventh library file was read inside the hour")
	case <-time.After(150 * time.Millisecond):
	}
	if maxIn.Load() != 1 {
		t.Fatalf("files read at once: %d", maxIn.Load())
	}
	wall.Store(start.Add(libraryWindow).UnixNano())
	select {
	case <-r.heard:
	case <-time.After(5 * time.Second):
		t.Fatal("the next file did not start once the hour had passed")
	}
}

func (r *queueRig) addEpisode(t *testing.T, title, path string, season, episode int) int64 {
	t.Helper()
	id, err := r.st.CreateRecording(context.Background(), store.Recording{
		Title: title, Path: path, Status: "complete",
		Season: season, Episode: episode, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func dirListing(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		names = append(names, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(names)
	return names
}
