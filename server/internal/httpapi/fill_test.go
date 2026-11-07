package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func probePTS(t *testing.T, body []byte) []float64 {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seg.ts")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "packet=pts_time", "-of", "csv=p=0", path).Output()
	if err != nil {
		t.Fatal(err)
	}
	var times []float64
	for _, line := range strings.Fields(string(out)) {
		if v, err := strconv.ParseFloat(strings.TrimSuffix(line, ","), 64); err == nil {
			times = append(times, v)
		}
	}
	sort.Float64s(times)
	return times
}

// After a resume, Start over and scrubbing back used to play a black, silent
// picture up to the resume point: the list covered it with gaps nothing
// encoded. Those are now fill segments, encoded when a player asks, and the
// copy the first viewer is in carries on.
func TestBeforeAResumePlaysThePicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not on PATH")
	}
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "show.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=s=320x180:r=30000/1001:d=20",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=20",
		"-c:v", "mpeg2video", "-b:v", "1500k", "-g", "15", "-bf", "2", "-flags", "+ilme+ildct", "-top", "1",
		"-c:a", "ac3", "-b:a", "96k", "-shortest", "-f", "mpegts", path)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	id, err := st.CreateRecording(t.Context(), store.Recording{
		Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetDuration(t.Context(), id, 20); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveProgress(t.Context(), id, 11.3); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: ffmpeg}
	t.Cleanup(func() { hub.Shutdown() })
	h := (&Server{Store: st, Hub: hub}).Handler()
	base := "/media/file/" + strconv.FormatInt(id, 10) + "/"
	play := func() float64 {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/recordings/"+strconv.FormatInt(id, 10)+"/play", strings.NewReader(`{}`)))
		if rec.Code != http.StatusOK {
			t.Fatal(rec.Code, rec.Body.String())
		}
		var body struct {
			Position float64 `json:"position"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Position
	}
	get := func(name string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+name, nil))
		return rec
	}
	if at := play(); at != 11.3 {
		t.Fatalf("resume at %v", at)
	}
	list := get("index.m3u8")
	if list.Code != http.StatusOK {
		t.Fatal(list.Code, list.Body.String())
	}
	body := list.Body.String()
	if strings.Contains(body, "#EXT-X-GAP") {
		t.Fatalf("the stretch before the resume is gaps nothing plays:\n%s", body)
	}
	var before []string
	for _, line := range strings.Split(body, "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "seg") {
			break
		}
		before = append(before, line)
	}
	// The copy starts on the two-second grid at or before the resume.
	if len(before) != 5 {
		t.Fatalf("%d segments before the copy:\n%s", len(before), body)
	}
	if !strings.Contains(body, before[4]+"\n#EXT-X-DISCONTINUITY\n") {
		t.Fatalf("no discontinuity between the two clocks:\n%s", body)
	}
	copyStart, err := os.Stat(filepath.Join(dir, "file", strconv.FormatInt(id, 10), "seg00000.ts"))
	if err != nil {
		t.Fatal(err)
	}

	// Start over: the first one has the first picture, in a few seconds.
	began := time.Now()
	first := get(before[0])
	took := time.Since(began)
	if first.Code != http.StatusOK {
		t.Fatalf("start over %d", first.Code)
	}
	pts := probePTS(t, first.Body.Bytes())
	if len(pts) < 30 || pts[0] < 1.3 || pts[0] > 1.5 || pts[len(pts)-1] > 3.45 {
		t.Fatalf("start over pictures %v", pts)
	}
	if took > 8*time.Second {
		t.Fatalf("start over took %v", took)
	}
	t.Logf("start over: first picture in %v", took.Round(time.Millisecond))
	// A scrub further on starts the fill encode there.
	began = time.Now()
	mid := get(before[3])
	took = time.Since(began)
	if mid.Code != http.StatusOK {
		t.Fatalf("scrub back %d", mid.Code)
	}
	if pts := probePTS(t, mid.Body.Bytes()); len(pts) < 30 || pts[0] < 7.3 || pts[0] > 7.5 {
		t.Fatalf("scrub back pictures %v", pts)
	}
	t.Logf("scrub back: picture in %v", took.Round(time.Millisecond))
	// The last one ends where the copy begins, give or take a picture or two.
	last := get(before[4])
	if pts := probePTS(t, last.Body.Bytes()); len(pts) < 30 || pts[len(pts)-1] > 11.5 {
		t.Fatalf("the last fill runs into the copy: %v", pts)
	}

	// A second viewer starting over doesn't replace the copy the first is in.
	if err := st.SaveProgress(t.Context(), id, 0); err != nil {
		t.Fatal(err)
	}
	if at := play(); at != 0 {
		t.Fatalf("second viewer at %v", at)
	}
	after, err := os.Stat(filepath.Join(dir, "file", strconv.FormatInt(id, 10), "seg00000.ts"))
	if err != nil {
		t.Fatalf("the copy went: %v", err)
	}
	if !after.ModTime().Equal(copyStart.ModTime()) {
		t.Fatal("the copy was encoded again")
	}
	if got := live.FileOffset(filepath.Join(dir, "file", strconv.FormatInt(id, 10))); got != 10 {
		t.Fatalf("copy offset %v", got)
	}
}
