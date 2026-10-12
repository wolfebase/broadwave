package dvr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestKeepRuleLeavesARecordingWhoseFileStays(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", KeepMode: "last", KeepCount: 1}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	stuck := filepath.Join(root, "stuck.ts")
	if err := os.MkdirAll(filepath.Join(stuck, "inside"), 0o755); err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(root, "gone.ts")
	if err := os.WriteFile(gone, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	fresh := filepath.Join(root, "fresh.ts")
	if err := os.WriteFile(fresh, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	stuckID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", Path: stuck, StartedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	goneID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", Path: gone, StartedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	freshID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", Path: fresh, StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, freshID)
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "no-ffmpeg")}
	OnSaved(ctx, st, hub, nil, rec)

	if _, err := st.Recording(ctx, stuckID); err != nil {
		t.Fatalf("row dropped while the file stayed: %v", err)
	}
	if _, err := os.Stat(stuck); err != nil {
		t.Fatalf("stuck file went: %v", err)
	}
	if _, err := st.Recording(ctx, goneID); err == nil {
		t.Fatal("a recording whose file was removed kept its row")
	}
	if _, err := os.Stat(gone); !os.IsNotExist(err) {
		t.Fatalf("removable file stayed: %v", err)
	}
	if _, err := st.Recording(ctx, freshID); err != nil {
		t.Fatalf("the recording that just finished went: %v", err)
	}
}

func TestKeepRuleDoesNotCountAMissingFile(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", KeepMode: "last", KeepCount: 1}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	missingID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", Path: filepath.Join(root, "missing.ts"), StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	olderPath := filepath.Join(root, "older.ts")
	if err := os.WriteFile(olderPath, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	olderID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", Path: olderPath, StartedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, olderID)
	if err != nil {
		t.Fatal(err)
	}
	// The newest row has no file. It must not count as the one episode kept,
	// or the real recording would be deleted to get down to one.
	OnSaved(ctx, st, &live.Hub{Dir: dir}, nil, rec)
	if _, err := st.Recording(ctx, missingID); err != nil {
		t.Fatalf("missing file lost its row: %v", err)
	}
	if _, err := st.Recording(ctx, olderID); err != nil {
		t.Fatalf("the only real episode was removed because a missing file took the slot: %v", err)
	}
	if _, err := os.Stat(olderPath); err != nil {
		t.Fatalf("older file: %v", err)
	}
}

// Saving one show must not apply another show's keep rule. The older News
// episode goes when a News episode is saved.
func TestKeepRuleWaitsForAnotherEpisodeOfThatShow(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	newsID, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", KeepMode: "last", KeepCount: 1})
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 11, 18, 0, 0, 0, time.UTC)
	write := func(name string) string {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	olderID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", PassID: newsID, Path: write("older.ts"), StartedAt: now.Add(-2 * time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	newerID, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Status: "complete", PassID: newsID, Path: write("newer.ts"), StartedAt: now.Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	movieID, err := st.CreateRecording(ctx, store.Recording{
		Title: "Movie", Status: "complete", Path: write("movie.ts"), StartedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	movie, err := st.Recording(ctx, movieID)
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Dir: dir}
	OnSaved(ctx, st, hub, nil, movie)
	for _, id := range []int64{olderID, newerID, movieID} {
		if _, err := st.Recording(ctx, id); err != nil {
			t.Fatalf("saving a movie removed recording %d: %v", id, err)
		}
	}

	newer, err := st.Recording(ctx, newerID)
	if err != nil {
		t.Fatal(err)
	}
	OnSaved(ctx, st, hub, nil, newer)
	if _, err := st.Recording(ctx, olderID); err == nil {
		t.Fatal("the older News episode stayed after a newer one was saved")
	}
	if _, err := os.Stat(filepath.Join(root, "older.ts")); !os.IsNotExist(err) {
		t.Fatalf("older News file stayed: %v", err)
	}
	if _, err := st.Recording(ctx, newerID); err != nil {
		t.Fatalf("the News episode that was saved went: %v", err)
	}
	if _, err := st.Recording(ctx, movieID); err != nil {
		t.Fatalf("the movie went when News was saved: %v", err)
	}
}
