package live

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/store"
)

func removeFixture(t *testing.T) (*Hub, store.Recording, string) {
	t.Helper()
	root := t.TempDir()
	h := &Hub{Dir: t.TempDir(), RecordingsDir: root}
	dir := filepath.Join(root, "TV", "Show")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "Show - 2026-10-08 - 1730.ts")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	return h, store.Recording{ID: 3, Path: path}, dir
}

func TestRemovingARecordingStopsItsScan(t *testing.T) {
	h, rec, dir := removeFixture(t)
	ctx, release := h.ReadingRecording(context.Background(), rec.ID)
	other, releaseOther := h.ReadingRecording(context.Background(), rec.ID+1)
	defer releaseOther()
	go func() {
		<-ctx.Done()
		release()
	}()
	if err := h.RemoveRecordingFiles(rec); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() == nil {
		t.Fatal("the scan of a removed recording is still running")
	}
	if other.Err() != nil {
		t.Fatal("removing one recording stopped another's scan")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the emptied folder stayed: %v", err)
	}
}

// A file still open on a FUSE share stays as a hidden file, so the folder only
// empties when the scan lets go. Its end tries the folder again.
func TestAScanThatOutlastsTheWaitPrunesWhenItEnds(t *testing.T) {
	prev := readerWait
	readerWait = 50 * time.Millisecond
	t.Cleanup(func() { readerWait = prev })
	h, rec, dir := removeFixture(t)
	hidden := filepath.Join(dir, ".fuse_hidden0001")
	if err := os.WriteFile(hidden, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, release := h.ReadingRecording(context.Background(), rec.ID)
	if err := h.RemoveRecordingFiles(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("the folder went while it still held a file: %v", err)
	}
	if err := os.Remove(hidden); err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("the emptied folder stayed: %v", err)
	}
}

// Pruning takes only what is truly empty, and only inside the recordings folder.
func TestPruningLeavesFoldersItDoesNotOwn(t *testing.T) {
	h, rec, dir := removeFixture(t)
	neighbor := filepath.Join(dir, "Other episode.ts")
	if err := os.WriteFile(neighbor, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	library := filepath.Join(outside, "Library", "Show")
	if err := os.MkdirAll(library, 0o755); err != nil {
		t.Fatal(err)
	}
	libraryFile := filepath.Join(library, "S01E01.ts")
	if err := os.WriteFile(libraryFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveRecordingFiles(rec); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(neighbor); err != nil {
		t.Fatalf("a neighbor went: %v", err)
	}
	if err := h.RemoveRecordingFiles(store.Recording{ID: 4, Path: libraryFile}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(libraryFile); err != nil {
		t.Fatalf("a library file went: %v", err)
	}
	empty := filepath.Join(outside, "Library", "Empty")
	if err := os.MkdirAll(empty, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := h.RemoveRecordingFiles(store.Recording{ID: 5, Path: filepath.Join(empty, "gone.ts")}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); err != nil {
		t.Fatalf("an empty folder outside the recordings folder went: %v", err)
	}
}
