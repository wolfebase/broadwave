package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestWriteNFORefreshesFinishedRecordingsInsideTheFolder(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	shelf := filepath.Join(dir, "shelf")
	if err := os.MkdirAll(root, 0o755); err != nil || os.MkdirAll(shelf, 0o755) != nil {
		t.Fatal(err)
	}
	show := filepath.Join(root, "Evening News.ts")
	movie := filepath.Join(shelf, "Movie.mp4")
	secret := filepath.Join(dir, "secret.ts")
	for _, path := range []string{show, movie, secret} {
		if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := testStore(t)
	ctx := context.Background()
	if _, err := st.AddSource(ctx, "folder", "Shelf", shelf, ""); err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 9, 28, 22, 0, 0, 0, time.UTC)
	idOf := func(path, title, status string) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{
			Title: title, Status: status, Path: path, StartedAt: start,
			Subtitle: "Local headlines", Description: "The evening newscast.", Category: "News",
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	showID := idOf(show, "Evening News", "complete")
	idOf(movie, "Movie", "complete")
	idOf(secret, "Secret", "complete")
	idOf(root+"/../secret.ts", "Escape", "complete")
	busy := filepath.Join(root, "busy.ts")
	if err := os.WriteFile(busy, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	idOf(busy, "Busy", "recording")

	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/recordings/nfo", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d %s", rec.Code, rec.Body.Bytes())
	}
	var body struct {
		Written int `json:"written"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Written != 1 {
		t.Fatalf("written %d %s", body.Written, rec.Body.Bytes())
	}
	nfoBody, err := os.ReadFile(filepath.Join(root, "Evening News.nfo"))
	if err != nil || !bytes.Contains(nfoBody, []byte("<showtitle>Evening News</showtitle>")) {
		t.Fatalf("nfo %v %s", err, nfoBody)
	}
	for _, path := range []string{
		filepath.Join(shelf, "Movie.nfo"),
		filepath.Join(dir, "secret.nfo"),
		filepath.Join(root, "busy.nfo"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("wrote %s: %v", path, err)
		}
	}

	// A second refresh rewrites the same file and still refuses the others.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/nfo", nil))
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"written":1`)) {
		t.Fatalf("refresh %d %s", rec.Code, rec.Body.Bytes())
	}

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/recordings/"+strconv.FormatInt(showID, 10), nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.Bytes())
	}
	if _, err := os.Stat(filepath.Join(root, "Evening News.nfo")); !os.IsNotExist(err) {
		t.Fatalf("nfo survived delete: %v", err)
	}
	if _, err := os.Stat(show); !os.IsNotExist(err) {
		t.Fatalf("recording survived delete: %v", err)
	}
}

func TestWriteNfoSetting(t *testing.T) {
	st := testStore(t)
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/settings")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"writeNfo":"0"`)) {
		t.Fatalf("default %s", res.Body.Bytes())
	}
	for _, raw := range []string{`{"writeNfo":"yes"}`, `{"writeNfo":"2"}`, `{"writeNfo":""}`} {
		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(raw))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s -> %d %s", raw, rec.Code, rec.Body.Bytes())
		}
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(`{"writeNfo":"1"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !bytes.Contains(rec.Body.Bytes(), []byte(`"writeNfo":"1"`)) {
		t.Fatalf("save %d %s", rec.Code, rec.Body.Bytes())
	}
}

func TestTurningNfoOnWritesFinishedRecordings(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	show := filepath.Join(root, "Evening News.ts")
	if err := os.WriteFile(show, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	if _, err := st.CreateRecording(context.Background(), store.Recording{
		Title: "Evening News", Status: "complete", Path: show, StartedAt: time.Now().Add(-time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewBufferString(`{"writeNfo":"1"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("save %d %s", rec.Code, rec.Body.Bytes())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "Evening News.nfo")); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("no nfo after turning the setting on")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
