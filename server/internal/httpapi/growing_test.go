package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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

func TestAGrowingRecordingReportsTheMediaOnDisk(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	raw = append(raw, clockTS(0)...)
	raw = append(raw, clockTS(10*90000)...)
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\ncat > index.m3u8 << 'EOF'\n#EXTM3U\n#EXTINF:2.000,\nseg00000.ts\nEOF\necho x > seg00000.ts\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-10 * time.Minute)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "recording", Path: path, StartedAt: started, Duration: 30 * 60,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, 30*60); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(t.Context(), id, 40); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: bin}}).Handler()
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil))
	if listRec.Code != http.StatusOK {
		t.Fatal(listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Recordings []struct {
			ID       int64   `json:"id"`
			Duration float64 `json:"durationSec"`
		} `json:"recordings"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	var listedDur float64
	for _, item := range listed.Recordings {
		if item.ID == id {
			listedDur = item.Duration
		}
	}
	if listedDur < 9 || listedDur > 11 {
		t.Fatalf("list duration %v, want the 10 seconds on disk", listedDur)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/play", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Position  float64 `json:"position"`
		Recording struct {
			Duration float64 `json:"durationSec"`
		} `json:"recording"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Recording.Duration < 9 || body.Recording.Duration > 11 || body.Position != 8 {
		t.Fatalf("play duration %v position %v, want about 10s starting at 8", body.Recording.Duration, body.Position)
	}
	off, err := os.ReadFile(filepath.Join(dir, "file", strconv.FormatInt(id, 10), "offset.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(off)) != "8.000" {
		t.Fatalf("offset %q", off)
	}
}

func TestALaterSeekStartsWhereTheViewerWent(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "night.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0x47, 0x00}, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, 3600); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(t.Context(), id, 10); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: playScript(t, dir)}}).Handler()
	if got := playDuration(t, h, id); got != 3600 {
		t.Fatalf("duration %v", got)
	}
	off := filepath.Join(dir, "file", strconv.FormatInt(id, 10), "offset.txt")
	body, err := os.ReadFile(off)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "10.000" {
		t.Fatalf("first offset %q", body)
	}
	if err := st.SaveProgress(t.Context(), id, 40*60); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/play", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var played struct {
		Position float64 `json:"position"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &played); err != nil {
		t.Fatal(err)
	}
	if played.Position != 40*60 {
		t.Fatalf("position %v", played.Position)
	}
	body, err = os.ReadFile(off)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(body)) != "2400.000" {
		t.Fatalf("second offset %q", body)
	}
	seg := httptest.NewRecorder()
	h.ServeHTTP(seg, httptest.NewRequest(http.MethodGet, "/media/file/"+strconv.FormatInt(id, 10)+"/seg00000.ts", nil))
	if seg.Code != http.StatusOK {
		t.Fatalf("segment %d %s", seg.Code, seg.Body.String())
	}
}

// playScript is an ffmpeg stand-in that writes a one-segment playlist and exits.
// Duration probes use the real ffprobe, not this binary.
func playScript(t *testing.T, dir string) string {
	t.Helper()
	bin := filepath.Join(dir, "ffmpeg")
	script := "#!/bin/sh\ncase \" $* \" in\n*\" -f hls \"*)\ncat > index.m3u8 << 'EOF'\n#EXTM3U\n#EXTINF:2.000,\nseg00000.ts\nEOF\necho x > seg00000.ts\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// oneSecondRecording is a real file so ffprobe can report a length.
func oneSecondRecording(t *testing.T, dir string) string {
	t.Helper()
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "recordings", "clip.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=32x18:rate=10:duration=1",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=8000:duration=1",
		"-c:v", "mpeg2video", "-c:a", "mp2", "-shortest", "-f", "mpegts", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	return path
}

func playDuration(t *testing.T, h http.Handler, id int64) float64 {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/play", strings.NewReader(`{}`)))
	if rec.Code != http.StatusOK {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var body struct {
		Recording struct {
			Duration float64 `json:"durationSec"`
		} `json:"recording"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Recording.Duration
}

func TestPlayKeepsAFailedDurationProbe(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := oneSecondRecording(t, dir)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, -1); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: playScript(t, dir)}}).Handler()
	if got := playDuration(t, h, id); got != -1 {
		t.Fatalf("duration %v, want the stored -1", got)
	}
	saved, err := st.Recording(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Duration != -1 {
		t.Fatalf("stored duration %v", saved.Duration)
	}
}

func TestPlayFillsInAMissingDuration(t *testing.T) {
	st := testStore(t)
	dir := t.TempDir()
	path := oneSecondRecording(t, dir)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: playScript(t, dir)}}).Handler()
	got := playDuration(t, h, id)
	if got < 0.5 || got > 5 {
		t.Fatalf("duration %v, want the file", got)
	}
	saved, err := st.Recording(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Duration != got {
		t.Fatalf("stored %v, response %v", saved.Duration, got)
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
