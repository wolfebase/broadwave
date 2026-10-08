package live

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"broadwave/internal/store"
)

func moveRig(t *testing.T) (*Hub, store.Recording) {
	t.Helper()
	h := &Hub{Dir: t.TempDir()}
	dir := filepath.Join(h.Recordings(), "old")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "20261005_193000_4.1_KBWV.ts")
	for _, ext := range []string{".ts", ".json", ".edl"} {
		if err := os.WriteFile(path[:len(path)-3]+ext, []byte(ext), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return h, store.Recording{ID: 1, Path: path, Status: "complete"}
}

func TestMoveRecordingTakesItsSidecars(t *testing.T) {
	h, rec := moveRig(t)
	got, err := h.MoveRecording(rec, " Harbor Watch / S01E02 The Lighthouse.ts ")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(h.Recordings(), "Harbor Watch", "S01E02 The Lighthouse.ts")
	if got != want {
		t.Fatalf("moved to %s, want %s", got, want)
	}
	for _, ext := range []string{".ts", ".json", ".edl"} {
		body, err := os.ReadFile(want[:len(want)-3] + ext)
		if err != nil || string(body) != ext {
			t.Fatalf("%s: %q %v", ext, body, err)
		}
	}
	if _, err := os.Stat(filepath.Dir(rec.Path)); !os.IsNotExist(err) {
		t.Fatalf("the emptied folder stayed (%v)", err)
	}
	if name := h.RecordingName(got); name != "Harbor Watch/S01E02 The Lighthouse.ts" {
		t.Fatalf("name %q", name)
	}
}

func TestMoveRecordingRefusesNamesOutsideTheFolder(t *testing.T) {
	h, rec := moveRig(t)
	for name, want := range map[string]error{
		"":                 ErrMoveName,
		"   ":              ErrMoveName,
		"/etc/passwd":      ErrMoveName,
		"../outside":       ErrMoveName,
		"show/../../x":     ErrMoveName,
		`..\x`:             ErrMoveName,
		"show//x":          ErrMoveName,
		".hidden":          ErrMoveName,
		"show/./x":         ErrMoveName,
		"Star Trek: Ships": ErrMoveChars,
		"a\x00b":           ErrMoveName,
		"line\nbreak":      ErrMoveName,
	} {
		if _, err := h.MoveRecording(rec, name); !errors.Is(err, want) {
			t.Errorf("%q: %v, want %v", name, err, want)
		}
	}
	if _, err := os.Stat(rec.Path); err != nil {
		t.Fatalf("a refused move touched the file: %v", err)
	}
}

func TestMoveRecordingKeepsOtherFiles(t *testing.T) {
	h, rec := moveRig(t)
	if err := os.WriteFile(filepath.Join(h.Recordings(), "taken.edl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.MoveRecording(rec, "taken"); !errors.Is(err, ErrMoveTaken) {
		t.Fatalf("over another recording's sidecar: %v", err)
	}
	library := store.Recording{ID: 2, Path: filepath.Join(t.TempDir(), "show.ts"), Status: "complete"}
	if _, err := h.MoveRecording(library, "show"); !errors.Is(err, ErrMoveOutside) {
		t.Fatalf("a library file: %v", err)
	}
}

func TestMoveRecordingDoesNotFollowALinkOut(t *testing.T) {
	h, rec := moveRig(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(h.Recordings(), "link")); err != nil {
		t.Fatal(err)
	}
	if _, err := h.MoveRecording(rec, "link/escaped"); !errors.Is(err, ErrMoveName) {
		t.Fatalf("through a link: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote outside: %v", entries)
	}
}

func TestMoveRecordingToItsOwnNameIsANoOp(t *testing.T) {
	h, rec := moveRig(t)
	got, err := h.MoveRecording(rec, "old/20261005_193000_4.1_KBWV")
	if err != nil || got != rec.Path {
		t.Fatalf("%s %v", got, err)
	}
}

func TestMoveRecordingChangesOnlyTheCase(t *testing.T) {
	h, rec := moveRig(t)
	dir := filepath.Dir(rec.Path)
	upper := filepath.Join(dir, "News.ts")
	if err := os.Rename(rec.Path, upper); err != nil {
		t.Fatal(err)
	}
	rec.Path = upper
	other := filepath.Join(dir, "news.ts")
	if err := os.WriteFile(other, []byte("another recording"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(upper)
	if string(body) == "another recording" {
		// The disk ignores case: news.ts is News.ts, and the rename is the file's own.
		if _, err := h.MoveRecording(rec, "old/news"); err != nil {
			t.Fatalf("case change on a case-blind disk: %v", err)
		}
		return
	}
	if _, err := h.MoveRecording(rec, "old/news"); !errors.Is(err, ErrMoveTaken) {
		t.Fatalf("replaced another recording: %v", err)
	}
	if got, _ := os.ReadFile(other); string(got) != "another recording" {
		t.Fatalf("other recording now %q", got)
	}
}

func TestMoveRecordingLeavesALinkedFolder(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	elsewhere := t.TempDir()
	if err := os.MkdirAll(h.Recordings(), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(h.Recordings(), "Sports")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(link, "game.ts")
	if err := os.WriteFile(path, []byte("ts"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := h.MoveRecording(store.Recording{ID: 1, Path: path, Status: "complete"}, "game"); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the linked folder went: %v", err)
	}
}
