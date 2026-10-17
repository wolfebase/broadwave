package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestARecordingStillAtATime(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	st := testStore(t)
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir, FFmpeg: "ffmpeg"}
	src := filepath.Join(hub.Recordings(), "show.ts")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30", "-t", "4",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-f", "mpegts", src)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	id, err := st.CreateRecording(context.Background(), store.Recording{Title: "Show", GuideNumber: "4.1", Status: "complete", Path: src, StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub}).Handler()
	base := "/api/v1/recordings/" + strconv.FormatInt(id, 10) + "/still"
	res := get(t, h, base+"?at=1.5&w=300")
	if res.Code != http.StatusOK || res.Header().Get("Content-Type") != "image/jpeg" || res.Body.Len() < 100 {
		t.Fatalf("still: %d %q %d bytes", res.Code, res.Header().Get("Content-Type"), res.Body.Len())
	}
	if _, err := os.Stat(live.StillPath(dir, id, 1.5, 480)); err != nil {
		t.Fatalf("not kept: %v", err)
	}
	status := func(path string) int {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code
	}
	for query, want := range map[string]int{"": 400, "?at=x": 400, "?at=-2": 400, "?at=90": 404} {
		if got := status(base + query); got != want {
			t.Errorf("%q: %d, want %d", query, got, want)
		}
	}
	if got := status("/api/v1/recordings/999/still?at=1"); got != 404 {
		t.Errorf("no recording: %d", got)
	}
	if scope := requiredScope(http.MethodGet, base); scope != store.ScopeWatch {
		t.Errorf("scope %q", scope)
	}
}
