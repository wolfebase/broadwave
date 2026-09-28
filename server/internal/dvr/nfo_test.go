package dvr

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestOnSavedWritesNFOOnlyInsideTheRecordingsFolder(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	show := filepath.Join(root, "Evening News.ts")
	if err := os.WriteFile(show, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []store.Airing{{
		ChannelID: 4, Title: "Evening News", ProgramID: "EP9", Subtitle: "Local headlines",
		Description: "The evening newscast.", Category: "News",
		Start: start, End: start.Add(time.Hour), Season: 3, Episode: 4, OriginalAir: "2024-11-02",
	}}); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: 4, Title: "Evening News", ProgramID: "EP9", Status: "complete",
		Path: show, StartedAt: start.Add(10 * time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "no-ffmpeg")}
	OnSaved(ctx, st, hub, rec)
	nfoPath := filepath.Join(root, "Evening News.nfo")
	if _, err := os.Stat(nfoPath); !os.IsNotExist(err) {
		t.Fatalf("wrote an nfo while the setting was off: %v", err)
	}
	if err := st.PutSettings(ctx, map[string]string{"writeNfo": "1"}); err != nil {
		t.Fatal(err)
	}
	OnSaved(ctx, st, hub, rec)
	body, err := os.ReadFile(nfoPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<showtitle>Evening News</showtitle>",
		"<title>Local headlines</title>",
		"<season>3</season>",
		"<episode>4</episode>",
		"<aired>2024-11-02</aired>",
		"<genre>News</genre>",
	} {
		if !bytes.Contains(body, []byte(want)) {
			t.Fatalf("missing %s in %s", want, body)
		}
	}

	secret := filepath.Join(dir, "secret.ts")
	if err := os.WriteFile(secret, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	outsideID, err := st.CreateRecording(ctx, store.Recording{
		Title: "Secret", Status: "complete", Path: secret, StartedAt: start,
	})
	if err != nil {
		t.Fatal(err)
	}
	outside, err := st.Recording(ctx, outsideID)
	if err != nil {
		t.Fatal(err)
	}
	OnSaved(ctx, st, hub, outside)
	if _, err := os.Stat(filepath.Join(dir, "secret.nfo")); !os.IsNotExist(err) {
		t.Fatalf("nfo left the recordings folder: %v", err)
	}
}
