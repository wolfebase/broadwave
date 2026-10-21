package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"broadwave/internal/live"
)

func TestImportFoundRecordings(t *testing.T) {
	dir := t.TempDir()
	st := testStore(t)
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	post := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/import", nil))
		return rec
	}
	if rec := post(); rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), "mounted") {
		t.Fatalf("no folder: %d %s", rec.Code, rec.Body.String())
	}
	root := filepath.Join(dir, "recordings")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "show.ts"), []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	side := `{"id": 9, "title": "Show", "startedAt": "2026-10-01T01:00:00Z"}`
	if err := os.WriteFile(filepath.Join(root, "show.json"), []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}
	if rec := post(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"added":1`) {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}
}

func TestRecordingRefusedWhenTheFolderIsNotMounted(t *testing.T) {
	dir := t.TempDir()
	st := testStore(t)
	root := filepath.Join(dir, "recordings")
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: filepath.Join(dir, "missing-ffmpeg")}
	if err := hub.RecordingsReady(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(root); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings", strings.NewReader(`{"channelId":1,"minutes":5,"title":"Nope"}`)))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"recordings_missing"`) {
		t.Fatalf("not mounted: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("the folder was made again: %v", err)
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/storage", nil))
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "mounted") {
		t.Fatalf("storage: %d %s", rec.Code, rec.Body.String())
	}
}
