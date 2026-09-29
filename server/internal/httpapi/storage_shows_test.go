package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestStorageShowsGroupsFinishedRecordingsInTheFolder(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	shelf := filepath.Join(dir, "shelf")
	if err := os.MkdirAll(root, 0o755); err != nil || os.MkdirAll(shelf, 0o755) != nil {
		t.Fatal(err)
	}
	older := time.Date(2026, 9, 28, 18, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	write := func(path string, n int) {
		t.Helper()
		if err := os.WriteFile(path, bytes.Repeat([]byte{0x47}, n), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	newsA := filepath.Join(root, "a.ts")
	newsB := filepath.Join(root, "b.ts")
	busy := filepath.Join(root, "busy.ts")
	failed := filepath.Join(root, "failed.ts")
	game := filepath.Join(root, "game.ts")
	library := filepath.Join(shelf, "library.ts")
	outside := filepath.Join(dir, "outside.ts")
	write(newsA, 100)
	write(newsB, 40)
	write(busy, 500)
	write(failed, 80)
	write(game, 200)
	write(library, 999)
	write(outside, 70)

	st := testStore(t)
	ctx := context.Background()
	add := func(path, title, status string, start time.Time) {
		t.Helper()
		if _, err := st.CreateRecording(ctx, store.Recording{
			ChannelID: 1, Title: title, Status: status, Path: path, StartedAt: start,
		}); err != nil {
			t.Fatal(err)
		}
	}
	add(newsA, "Evening News", "complete", older)
	add(newsB, "evening news", "complete", newer)
	add(busy, "Evening News", "recording", newer.Add(time.Hour))
	add(failed, "Evening News", "failed", older)
	add(library, "Evening News", "complete", newer)
	add(outside, "Evening News", "complete", newer)
	add(filepath.Join(root, "..", "outside.ts"), "Evening News", "complete", newer)
	add(game, "The Afternoon Game", "complete", older)

	if err := st.AddPass(ctx, "Evening News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(ctx, "Other", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	passes, err := st.Passes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var newsPass store.Pass
	for _, p := range passes {
		if p.Title == "Evening News" {
			newsPass = p
		}
	}
	if newsPass.ID == 0 {
		t.Fatal("missing pass")
	}
	newsPass.KeepMode = "last"
	newsPass.KeepCount = 3
	if err := st.UpdatePassRules(ctx, newsPass); err != nil {
		t.Fatal(err)
	}

	unconfigured := httptest.NewRecorder()
	(&Server{Store: st}).Handler().ServeHTTP(unconfigured, httptest.NewRequest(http.MethodGet, "/api/v1/storage/shows", nil))
	if unconfigured.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured %d %s", unconfigured.Code, unconfigured.Body.Bytes())
	}

	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/storage/shows", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.Bytes())
	}
	var body struct {
		Shows []storageShow `json:"shows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Shows) != 2 {
		t.Fatalf("shows %#v", body.Shows)
	}
	if body.Shows[0].Title != "The Afternoon Game" || body.Shows[0].Bytes != 200 || body.Shows[0].Count != 1 || body.Shows[0].Pass != nil {
		t.Fatalf("largest %#v", body.Shows[0])
	}
	news := body.Shows[1]
	if news.Title != "evening news" || news.Count != 2 || news.Bytes != 140 {
		t.Fatalf("news %#v", news)
	}
	if !news.Oldest.Equal(older) || !news.Newest.Equal(newer) {
		t.Fatalf("span %s %s", news.Oldest, news.Newest)
	}
	if news.Pass == nil || news.Pass.ID != newsPass.ID || news.Pass.Keep != 3 {
		t.Fatalf("pass %#v", news.Pass)
	}
}
