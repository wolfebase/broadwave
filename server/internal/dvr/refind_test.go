package dvr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func forgetWalks(t *testing.T) {
	lostMu.Lock()
	lastLost, lastWalk = "", time.Time{}
	lostMu.Unlock()
}

func TestRefindFollowsAFileMovedByHand(t *testing.T) {
	forgetWalks(t)
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	hub := &live.Hub{Dir: t.TempDir()}
	started := time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC)
	add := func(title, name string) int64 {
		id, err := st.CreateRecording(ctx, store.Recording{Title: title, Status: "complete", StartedAt: started, Path: filepath.Join(hub.Recordings(), name)})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	moved := add("Harbor Watch", "20261005_193000_4.1_KBWV.ts")
	other := add("Night Court", "20261005_193000_5.1_WTST.ts")
	// The moved file and its .json, with the start as the server writes it.
	place := func(dir, base string, id int64, title string) string {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		media := filepath.Join(dir, base+".ts")
		side, _ := json.Marshal(map[string]any{"id": id, "title": title, "startedAt": started.Add(400 * time.Millisecond)})
		if err := os.WriteFile(media, []byte("ts"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, base+".json"), side, 0o644); err != nil {
			t.Fatal(err)
		}
		return media
	}
	media := place(filepath.Join(hub.Recordings(), "Harbor Watch", "Season 1"), "S01E02", moved, "Harbor Watch")
	// A .json that names the other recording's id with a different title (a
	// restored catalog reuses ids) is not followed.
	place(filepath.Join(hub.Recordings(), "Imposter"), "x", other, "Someone Else")

	Refind(ctx, st, hub)
	rec, err := st.Recording(ctx, moved)
	if err != nil || rec.Path != media {
		t.Fatalf("moved file: %q %v", rec.Path, err)
	}
	if rec, _ := st.Recording(ctx, other); filepath.Base(rec.Path) != "20261005_193000_5.1_WTST.ts" {
		t.Fatalf("followed an imposter: %s", rec.Path)
	}
	events, _ := st.Events(ctx, 5)
	if len(events) == 0 || events[0].Message != "Found Harbor Watch at Harbor Watch/Season 1/S01E02.ts" {
		t.Fatalf("events %+v", events)
	}
}

func TestRefindTakesNoLinksAndFollowsALinkedFolder(t *testing.T) {
	forgetWalks(t)
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	real := t.TempDir()
	link := filepath.Join(t.TempDir(), "recordings")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Dir: t.TempDir(), RecordingsDir: link}
	started := time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC)
	add := func(title string) int64 {
		id, err := st.CreateRecording(ctx, store.Recording{Title: title, Status: "complete", StartedAt: started, Path: filepath.Join(link, title+".ts")})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	side := func(dir, base string, id int64, title string) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		body, _ := json.Marshal(map[string]any{"id": id, "title": title, "startedAt": started})
		if err := os.WriteFile(filepath.Join(dir, base+".json"), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	plain := add("plain")
	linked := add("linked")
	side(filepath.Join(real, "moved"), "plain", plain, "plain")
	if err := os.WriteFile(filepath.Join(real, "moved", "plain.ts"), []byte("ts"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A .ts that is a link to a file elsewhere is not a recording.
	secret := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(secret, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	side(filepath.Join(real, "trap"), "linked", linked, "linked")
	if err := os.Symlink(secret, filepath.Join(real, "trap", "linked.ts")); err != nil {
		t.Fatal(err)
	}

	Refind(ctx, st, hub)
	if rec, _ := st.Recording(ctx, plain); rec.Path != filepath.Join(link, "moved", "plain.ts") {
		t.Fatalf("plain: %s", rec.Path)
	}
	if rec, _ := st.Recording(ctx, linked); rec.Path != filepath.Join(link, "linked.ts") {
		t.Fatalf("followed a link: %s", rec.Path)
	}
}
