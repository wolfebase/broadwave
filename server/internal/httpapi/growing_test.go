package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestARecordingInProgressReportsHowLongItHasRun(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 188), 0o644); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-10 * time.Minute)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "recording", Path: path, StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Recordings []struct {
			ID       int64   `json:"id"`
			Duration float64 `json:"durationSec"`
		} `json:"recordings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	var got float64
	for _, item := range body.Recordings {
		if item.ID == id {
			got = item.Duration
		}
	}
	if got < 500 || got > 700 {
		t.Fatalf("duration %v, want about 10 minutes", got)
	}
}

func TestPlayOfAGrowingRecordingResumesInsideIt(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, 188*8), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\ncat > index.m3u8 << 'EOF'\n#EXTM3U\n#EXTINF:2.000,\nseg00000.ts\nEOF\necho x > seg00000.ts\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-10 * time.Minute)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "recording", Path: path, StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(t.Context(), id, 40); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: bin}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/play", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Growing   bool    `json:"growing"`
		Position  float64 `json:"position"`
		Recording struct {
			Duration float64 `json:"durationSec"`
		} `json:"recording"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.Growing || body.Position != 40 {
		t.Fatalf("play %+v", body)
	}
	if body.Recording.Duration < 500 || body.Recording.Duration > 700 {
		t.Fatalf("duration %v", body.Recording.Duration)
	}
	off, err := os.ReadFile(filepath.Join(dir, "file", strconv.FormatInt(id, 10), "offset.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(off)) != "40.000" {
		t.Fatalf("offset %q", off)
	}
}

func TestAReplacedRecordingSegmentIsNotCached(t *testing.T) {
	dir := t.TempDir()
	play := filepath.Join(dir, "file", "3")
	if err := os.MkdirAll(play, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(play, "seg00000.ts"), []byte("segment"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/file/3/seg00000.ts", nil))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-cache" {
		t.Fatalf("cache %q", got)
	}
}
