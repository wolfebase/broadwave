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
	"strings"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
)

// A player that switches sound in place gets the master playlist of an
// encode that carries the program's other sound track; every view it lists
// is served. A player that does not say so keeps the one-sound playlist.
func TestAnAlternatesWatchGetsTheMaster(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("ffmpeg not installed")
	}
	ctx := context.Background()
	sample := filepath.Join(t.TempDir(), "two-sounds.ts")
	gen := exec.Command(ffmpeg, "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=640x360:rate=30000/1001",
		"-f", "lavfi", "-i", "sine=frequency=500", "-f", "lavfi", "-i", "sine=frequency=900",
		"-map", "0", "-map", "1", "-map", "2", "-t", "10",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "30", "-pix_fmt", "yuv420p", "-c:a", "ac3",
		"-metadata:s:a:0", "language=eng", "-metadata:s:a:1", "language=spa", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
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
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg", Alternates: true}
	t.Cleanup(hub.Shutdown)
	h := (&Server{Store: st, HDHR: client, Hub: hub}).Handler()
	id := guideID(t, st, "4.1")

	watch := func(alternates bool) (string, string) {
		t.Helper()
		caps := fmt.Sprintf(`{"platform":"web","video":["h264"],"audio":["aac"],"alternates":%t}`, alternates)
		res := postJSON(t, h, "/api/v1/watch", fmt.Sprintf(`{"channelId":%d,"caps":%s}`, id, caps))
		if res.Code != http.StatusOK {
			t.Fatalf("watch %d %s", res.Code, res.Body.String())
		}
		var s struct {
			MainPlaylist string `json:"mainPlaylist"`
			Rendition    string `json:"rendition"`
		}
		if err := json.Unmarshal(res.Body.Bytes(), &s); err != nil {
			t.Fatal(err)
		}
		return s.MainPlaylist, s.Rendition
	}
	get := func(path string) string {
		t.Helper()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Body.String())
		}
		return rec.Body.String()
	}

	master, key := watch(true)
	want := fmt.Sprintf("/media/live/%d/%s/master.m3u8", id, key)
	if master != want {
		t.Fatalf("mainPlaylist %q, want %q", master, want)
	}
	body := get(master)
	if !strings.Contains(body, `LANGUAGE="es"`) {
		t.Fatalf("no Spanish alternate:\n%s", body)
	}
	dir := strings.TrimSuffix(master, "master.m3u8")
	uris := regexp.MustCompile(`URI="(audio-\d+\.m3u8)"`).FindAllStringSubmatch(body, -1)
	if len(uris) < 2 {
		t.Fatalf("want two sound views:\n%s", body)
	}
	for _, u := range append(uris, []string{"", "video.m3u8"}) {
		view := get(dir + u[1])
		m := regexp.MustCompile(`#EXT-X-MAP:URI="([^"]+)"`).FindStringSubmatch(view)
		if m == nil {
			t.Fatalf("%s has no init:\n%s", u[1], view)
		}
		get(dir + m[1])
	}

	// The same rendition, asked for by a player that reloads to switch.
	main, again := watch(false)
	if again != key || strings.HasSuffix(main, "master.m3u8") {
		t.Fatalf("legacy watch got %q on %s", main, again)
	}
}
