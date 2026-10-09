package dvr

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"broadwave/internal/disk"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

type storageRig struct {
	st  *store.Store
	hub *live.Hub
	dir string
	cfg string
}

func newStorageRig(t *testing.T, settings map[string]string) *storageRig {
	t.Helper()
	cfg := filepath.Join(t.TempDir(), "cfg")
	st, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.PutSettings(context.Background(), settings); err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	dir := filepath.Join(work, "recordings")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &storageRig{st: st, hub: &live.Hub{Dir: work}, dir: dir, cfg: cfg}
}

// add makes a finished recording with a 100-byte file and .edl/.json beside it.
// watched: 0 not watched, 1 marked watched, 2 watched to the end by playhead,
// 3 stopped at 94% by playhead.
func (r *storageRig) add(t *testing.T, title string, watched int, keep bool, path string) store.Recording {
	return r.addAs(t, title, watched, keep, path, "complete")
}

func (r *storageRig) addAs(t *testing.T, title string, watched int, keep bool, path, status string) store.Recording {
	t.Helper()
	ctx := context.Background()
	if path == "" {
		path = filepath.Join(r.dir, title+".ts")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, path[:len(path)-3] + ".edl", path[:len(path)-3] + ".json"} {
		if err := os.WriteFile(name, make([]byte, 100), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	id, err := r.st.CreateRecording(ctx, store.Recording{Title: title, Path: path, Status: "recording", StartedAt: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if status != "recording" {
		if err := r.st.FinishRecording(ctx, id, status, ""); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.st.SetDuration(ctx, id, 1800); err != nil {
		t.Fatal(err)
	}
	switch watched {
	case 1:
		err = r.st.SetWatched(ctx, id, 1)
	case 2:
		err = r.st.SaveProgress(ctx, id, 1795)
	case 3:
		err = r.st.SaveProgress(ctx, id, 1700)
	}
	if err != nil {
		t.Fatal(err)
	}
	if keep {
		if err := r.st.SetKeep(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}
	return mustRecording(t, r.st, id)
}

func (r *storageRig) left(t *testing.T) []string {
	t.Helper()
	recs, err := r.st.Recordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, rec := range recs {
		out = append(out, rec.Title)
	}
	slices.Sort(out)
	return out
}

func TestWatchedRecordingsGoAfterTheirDays(t *testing.T) {
	r := newStorageRig(t, map[string]string{"deleteWatchedDays": "7"})
	marked := r.add(t, "marked", 1, false, "")
	r.add(t, "played", 2, false, "")
	r.add(t, "unwatched", 0, false, "")
	r.add(t, "kept", 1, true, "")
	r.add(t, "library", 1, false, filepath.Join(t.TempDir(), "shows", "library.ts"))
	r.add(t, "stopped short", 3, false, "")
	r.addAs(t, "failed", 1, false, "", "failed")
	r.addAs(t, "on now", 1, false, "", "recording")
	ctx := context.Background()

	Tidy(ctx, r.st, r.hub, time.Now().Add(6*24*time.Hour))
	if got := r.left(t); len(got) != 8 {
		t.Fatalf("six days after watching, removed some: %v", got)
	}

	Tidy(ctx, r.st, r.hub, time.Now().Add(8*24*time.Hour))
	if got := r.left(t); !slices.Equal(got, []string{"failed", "kept", "library", "on now", "stopped short", "unwatched"}) {
		t.Fatalf("left %v", got)
	}
	for _, name := range []string{marked.Path, filepath.Join(r.dir, "marked.edl"), filepath.Join(r.dir, "marked.json")} {
		if _, err := os.Stat(name); !os.IsNotExist(err) {
			t.Fatalf("%s still there (%v)", name, err)
		}
	}
	events, _ := r.st.Events(ctx, 10)
	found := false
	for _, ev := range events {
		if ev.Message == "Removed marked, watched more than 7 days ago" {
			found = true
		}
	}
	if !found {
		t.Fatalf("no event: %+v", events)
	}
}

func TestWatchedRecordingsStayWhenTheSettingIsOff(t *testing.T) {
	r := newStorageRig(t, map[string]string{})
	r.add(t, "marked", 1, false, "")
	Tidy(context.Background(), r.st, r.hub, time.Now().Add(400*24*time.Hour))
	if got := r.left(t); len(got) != 1 {
		t.Fatalf("left %v", got)
	}
}

func TestRewatchingFromTheStartIsKept(t *testing.T) {
	r := newStorageRig(t, map[string]string{"deleteWatchedDays": "7"})
	rec := r.add(t, "rewatching", 1, false, "")
	ctx := context.Background()
	marked := time.Now().Add(-10 * 24 * time.Hour).UTC().Format(time.RFC3339)
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(r.cfg, "broadwave.db"))+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE recordings SET watched_at = ? WHERE id = ?`, marked, rec.ID); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := r.st.SaveProgress(ctx, rec.ID, 0); err != nil {
		t.Fatal(err)
	}
	Tidy(ctx, r.st, r.hub, time.Now())
	if got := r.left(t); !slices.Contains(got, "rewatching") {
		t.Fatalf("started over from the beginning and was removed: %v", got)
	}
}

func TestPlayingAgainStartsTheClockOver(t *testing.T) {
	marked := time.Date(2026, 9, 1, 20, 0, 0, 0, time.UTC)
	played := marked.Add(5 * 24 * time.Hour)
	rec := store.Recording{Watched: 1, WatchedAt: &marked, ProgressAt: &played, Position: 10, Duration: 1800}
	if got := rec.WatchedSince(); !got.Equal(played) {
		t.Fatalf("watched since %v, want the later play %v", got, played)
	}
	rec.Watched = 2
	if got := rec.WatchedSince(); !got.IsZero() {
		t.Fatalf("marked unwatched reads watched since %v", got)
	}
}

func TestMakeRoomTakesTheOldestWatchedFirst(t *testing.T) {
	r := newStorageRig(t, map[string]string{"makeRoom": "1"})
	r.add(t, "unwatched", 0, false, "")
	r.add(t, "kept", 1, true, "")
	r.add(t, "older", 1, false, "")
	r.add(t, "newer", 1, false, "")
	r.add(t, "newest", 1, false, "")
	r.add(t, "playing", 2, false, "")
	// Free space never moves, as on a filesystem that reports it late:
	// the files removed are counted instead.
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 1000, Total: 10000}, nil }
	MakeRoom(context.Background(), r.st, r.hub, 1150)
	if got := r.left(t); !slices.Equal(got, []string{"kept", "newest", "playing", "unwatched"}) {
		t.Fatalf("left %v", got)
	}
}

func TestMakeRoomRemovesNothingWhenItCannotFreeEnough(t *testing.T) {
	r := newStorageRig(t, map[string]string{"makeRoom": "1"})
	r.add(t, "watched", 1, false, "")
	r.add(t, "also watched", 1, false, "")
	r.add(t, "unwatched", 0, false, "")
	r.add(t, "playing", 2, false, "")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 1000, Total: 10000}, nil }
	MakeRoom(context.Background(), r.st, r.hub, 1201)
	if got := r.left(t); len(got) != 4 {
		t.Fatalf("left %v", got)
	}
	events, _ := r.st.Events(context.Background(), 5)
	if len(events) == 0 || events[0].Message != "Free space is under the reserve, and deleting every watched recording would not free enough" {
		t.Fatalf("events %+v", events)
	}
}

func TestMakeRoomReportsBytesUnlinked(t *testing.T) {
	r := newStorageRig(t, map[string]string{"makeRoom": "1"})
	r.add(t, "watched", 1, false, "")
	r.add(t, "also watched", 1, false, "")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 1000, Total: 10000}, nil }
	// Each recording file is 100 bytes. The sidecars are not counted.
	if got := MakeRoom(context.Background(), r.st, r.hub, 1150); got != 200 {
		t.Fatalf("freed %d, want the two recording files", got)
	}
}

func TestMakeRoomDoesNotCreditAFileItCouldNotUnlink(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can unlink a file in a mode 0555 directory")
	}
	r := newStorageRig(t, map[string]string{"makeRoom": "1"})
	rec := r.add(t, "watched", 1, false, "")
	if err := os.Chmod(r.dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(r.dir, 0o755) })
	if f, err := os.CreateTemp(r.dir, ".probe-*"); err == nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
		t.Skip("this user can still write a mode 0555 directory")
	}
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 1000, Total: 10000}, nil }
	if got := MakeRoom(context.Background(), r.st, r.hub, 1050); got != 0 {
		t.Fatalf("credited %d bytes for a file that is still there", got)
	}
	if _, err := os.Stat(rec.Path); err != nil {
		t.Fatalf("file: %v", err)
	}
	if got := r.left(t); !slices.Contains(got, "watched") {
		t.Fatalf("row removed: %v", got)
	}
}

func TestMakeRoomIsOffByDefault(t *testing.T) {
	r := newStorageRig(t, map[string]string{})
	r.add(t, "watched", 1, false, "")
	orig := diskStat
	t.Cleanup(func() { diskStat = orig })
	diskStat = func(string) (disk.Space, error) { return disk.Space{Free: 0, Total: 100}, nil }
	MakeRoom(context.Background(), r.st, r.hub, 10)
	if got := r.left(t); len(got) != 1 {
		t.Fatalf("left %v", got)
	}
}

func TestRoomOrderIsWatchedLongestAgoFirst(t *testing.T) {
	at := func(days int) *time.Time {
		v := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(days) * 24 * time.Hour)
		return &v
	}
	recs := []store.Recording{
		{ID: 1, Watched: 1, WatchedAt: at(3)},
		{ID: 2, Watched: 0, Position: 1790, Duration: 1800, ProgressAt: at(1)},
		{ID: 3, Watched: 0, Position: 100, Duration: 1800, ProgressAt: at(0)},
		{ID: 4, Watched: 1, WatchedAt: at(2)},
	}
	recs = append(recs, store.Recording{ID: 5, Watched: 0, Position: 1700, Duration: 1800, ProgressAt: at(-5)})
	var ids []int64
	for _, rec := range RoomOrder(recs) {
		ids = append(ids, rec.ID)
	}
	if !slices.Equal(ids, []int64{2, 4, 1}) {
		t.Fatalf("order %v", ids)
	}
}

func TestCleanUpFollowsTheRecordingsFolder(t *testing.T) {
	r := newStorageRig(t, map[string]string{"deleteWatchedDays": "1"})
	r.hub.RecordingsDir = filepath.Join(t.TempDir(), "elsewhere")
	r.add(t, "default folder", 1, false, "")
	moved := r.add(t, "elsewhere", 1, false, filepath.Join(r.hub.RecordingsDir, "elsewhere.ts"))
	Tidy(context.Background(), r.st, r.hub, time.Now().Add(2*24*time.Hour))
	if got := r.left(t); !slices.Equal(got, []string{"default folder"}) {
		t.Fatalf("left %v", got)
	}
	if _, err := os.Stat(moved.Path); !os.IsNotExist(err) {
		t.Fatalf("file still there (%v)", err)
	}
}
