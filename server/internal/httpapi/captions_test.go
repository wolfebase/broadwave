package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// AVPlayer asks the captions playlist for the video's next segment with
// _HLS_msn. Answering at once without it is a protocol error it logs as
// "Invalid server blocking reload behavior".
func TestACaptionsBlockingReloadWaitsForItsSegment(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ctx := context.Background()
	// Long enough that its loop, a timestamp break that restarts the
	// encode, comes after the test.
	sample := filepath.Join(t.TempDir(), "sample.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30:duration=20",
		"-f", "lavfi", "-i", "sine=frequency=440:sample_rate=48000:duration=20",
		"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "30",
		"-c:a", "aac", "-shortest", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	// Played at its own pace, so the next segment is not out yet.
	tuner := &fake.Server{TS: sample, Source: sample}
	base, control, err := tuner.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tuner.Close)
	t.Setenv("HDHR_CONTROL_PORT", control)
	st := testStore(t)
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	channels, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, channels); err != nil {
		t.Fatal(err)
	}
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, HDHR: client, Hub: hub}).Handler()

	res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d,"caps":{"platform":"web","video":["h264"],"audio":["aac"]}}`, guideID(t, st, "4.1")))
	if res.Code != http.StatusOK {
		t.Fatalf("watch %d %s", res.Code, res.Body.String())
	}
	var w struct {
		Playlist string `json:"playlist"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &w); err != nil {
		t.Fatal(err)
	}
	captions := strings.TrimSuffix(w.Playlist, "index.m3u8") + "captions.m3u8"
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}
	segs := regexp.MustCompile(`(?m)^seg(\d+)\.vtt$`)
	last := func(body string) int {
		found := segs.FindAllStringSubmatch(body, -1)
		if len(found) == 0 {
			return -1
		}
		n, _ := strconv.Atoi(found[len(found)-1][1])
		return n
	}
	var body string
	for end := time.Now().Add(20 * time.Second); last(body) < 0; time.Sleep(100 * time.Millisecond) {
		if time.Now().After(end) {
			t.Fatalf("no captions segment:\n%s", body)
		}
		body = get(captions)
	}
	if !strings.Contains(body, "#EXT-X-SERVER-CONTROL:CAN-BLOCK-RELOAD=YES\n") {
		t.Fatalf("captions do not say they block:\n%s", body)
	}
	next := last(body) + 1
	body = get(fmt.Sprintf("%s?_HLS_msn=%d", captions, next))
	if last(body) < next {
		t.Fatalf("asked for segment %d, got:\n%s\nvideo:\n%s", next, body, get(w.Playlist))
	}
}
