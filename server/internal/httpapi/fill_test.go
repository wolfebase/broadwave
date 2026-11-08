package httpapi

import (
	"context"
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

// A fill that can't be made was answered with a few empty packets, which a
// player told "2 s of media" can stall or fail on. It is an error now, and a
// player that gives up on a fill doesn't leave the request waiting.
func TestAFillThatCantBeMadeIsAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "show.ts")
	if err := os.WriteFile(path, make([]byte, 188*10), 0o644); err != nil {
		t.Fatal(err)
	}
	// A copy that waits, and a fill encode that never writes.
	bin := filepath.Join(dir, "ffmpeg")
	pids := filepath.Join(dir, "pids")
	script := "#!/bin/sh\necho $$ >> " + pids + "\ncase \" $* \" in\n*fill%05d.ts*) exec sleep 60 ;;\n" +
		"*\" -f hls \"*)\nprintf '#EXTM3U\\n#EXT-X-PLAYLIST-TYPE:EVENT\\n#EXTINF:2.000,\\nseg00000.ts\\n' > index.m3u8\necho x > seg00000.ts\nexec sleep 60\n;;\nesac\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		raw, _ := os.ReadFile(pids)
		for _, f := range strings.Fields(string(raw)) {
			if pid, _ := strconv.Atoi(f); pid > 0 {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	})
	hub := &live.Hub{Dir: dir, Encoder: "libx264", FFmpeg: bin}
	t.Cleanup(hub.Shutdown)
	if _, err := hub.PlayFile(4, path, "mpeg2video", "broadcast", "progressive", 21); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: hub}).Handler()

	// A recording with no copy: nothing can make its fill.
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/file/9/fill00001.ts", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("a fill nothing can make was served: %d bytes", rec.Body.Len())
	}

	// A player that leaves: the request ends with it.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	began := time.Now()
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/file/4/fill00002.ts", nil).WithContext(ctx))
	if took := time.Since(began); took > 2*time.Second {
		t.Fatalf("a request the player left waited %v", took)
	}
	if rec.Code == http.StatusOK {
		t.Fatalf("a fill that never came was served: %d bytes", rec.Body.Len())
	}
}

// A copy made before fill.json was kept had no way to make its fill segments,
// so a player holding its list got 404s until it pressed play again. The
// copy's own files and the catalog say enough to make them.
func TestAnOlderCopyStillFills(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not on PATH")
	}
	st := testStore(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "recordings", "show.ts")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	build := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=s=320x180:r=30000/1001:d=12",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=12",
		"-c:v", "mpeg2video", "-b:v", "1500k", "-g", "15", "-c:a", "ac3", "-b:a", "96k", "-shortest", "-f", "mpegts", path)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	id, err := st.CreateRecording(t.Context(), store.Recording{Title: "Night Shift", Status: "complete", Path: path, StartedAt: time.Now().Add(-time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	first := &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: ffmpeg}
	if _, err := first.PlayFile(id, path, "mpeg2video", "broadcast", "", 6.5); err != nil {
		t.Fatal(err)
	}
	first.Shutdown()
	copyDir := filepath.Join(dir, "file", strconv.FormatInt(id, 10))
	if err := os.Remove(filepath.Join(copyDir, "fill.json")); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: dir, Encoder: "libx264", FFmpeg: ffmpeg}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, Hub: hub}).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/media/file/"+strconv.FormatInt(id, 10)+"/fill00001.ts", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("an older copy's fill: %d", rec.Code)
	}
	if pts := probePTS(t, rec.Body.Bytes()); len(pts) < 30 || pts[0] < 3.3 || pts[0] > 3.5 {
		t.Fatalf("fill pictures %v", pts)
	}
}
