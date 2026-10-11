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
