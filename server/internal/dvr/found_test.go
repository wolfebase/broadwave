package dvr

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestImportFoundAddsRecordingsTheCatalogLacks(t *testing.T) {
	forgetWalks(t)
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D1", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "FOUR"}}); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Dir: t.TempDir()}
	root := hub.Recordings()
	started := time.Date(2026, 10, 5, 19, 30, 0, 0, time.UTC)
	ended := started.Add(30 * time.Minute)
	write := func(rel string, side map[string]any, media bool) string {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if side != nil {
			body, _ := json.Marshal(side)
			if err := os.WriteFile(path+".json", body, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if media {
			if err := os.WriteFile(path+".ts", []byte("ts"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return path + ".ts"
	}
	owned := write("Known/known", map[string]any{"id": 1, "title": "Known", "startedAt": started}, true)
	if _, err := st.CreateRecording(ctx, store.Recording{Title: "Known", Status: "complete", StartedAt: started, Path: owned}); err != nil {
		t.Fatal(err)
	}
	found := write("Harbor Watch/S01E02", map[string]any{"id": 41, "title": "Harbor Watch", "guideNumber": "4.1", "status": "complete", "startedAt": started, "endedAt": ended}, true)
	cut := write("Late/late", map[string]any{"id": 42, "title": "Late", "guideNumber": "9.9", "status": "interrupted", "startedAt": started}, true)
	write("Gone/gone", map[string]any{"id": 43, "title": "Gone", "startedAt": started}, false)
	write("Loose/loose", nil, true)

	n, err := ImportFound(ctx, st, hub)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("added %d, want 2", n)
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]store.Recording{}
	for _, rec := range recs {
		byPath[rec.Path] = rec
	}
	if len(recs) != 3 {
		t.Fatalf("recordings: %+v", recs)
	}
	harbor := byPath[found]
	if harbor.Title != "Harbor Watch" || harbor.Status != "complete" || harbor.ChannelID == 0 || harbor.EndedAt == nil || !harbor.EndedAt.Equal(ended) {
		t.Fatalf("harbor: %+v", harbor)
	}
	late := byPath[cut]
	if late.Status != "interrupted" || !late.Partial || late.ChannelID != 0 {
		t.Fatalf("late: %+v", late)
	}
	side, ok := readSidecar(filepath.Join(root, "Harbor Watch", "S01E02.json"))
	if !ok || side.ID != harbor.ID {
		t.Fatalf("sidecar id %d, want %d", side.ID, harbor.ID)
	}
	if n, err := ImportFound(ctx, st, hub); err != nil || n != 0 {
		t.Fatalf("second pass added %d (%v)", n, err)
	}
}
