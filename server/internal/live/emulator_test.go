package live

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// Threadfin, xTeVe, Antennas, ErsatzTV, and Dispatcharr answer discover.json
// and lineup.json and stream /auto/v<number>. None has status.json or the
// control port. A watch still plays.
func TestAnHDHomeRunEmulatorPlays(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/sample.ts"
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30", "-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "4", "-c:v", "libx264", "-preset", "ultrafast", "-g", "15", "-pix_fmt", "yuv420p", "-c:a", "aac", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/discover.json":
			_, _ = io.WriteString(w, `{"FriendlyName":"Threadfin","ModelNumber":"HDTC-2US","FirmwareName":"hdhomeruntc_atsc","DeviceID":"EMU00001","BaseURL":"`+srv.URL+`","LineupURL":"`+srv.URL+`/lineup.json","TunerCount":1}`)
		case r.URL.Path == "/lineup.json":
			_, _ = io.WriteString(w, `[{"GuideNumber":"1","GuideName":"News","URL":"`+srv.URL+`/auto/v1"},{"GuideNumber":"2","GuideName":"Sports","URL":"`+srv.URL+`/auto/v2"}]`)
		case strings.HasPrefix(r.URL.Path, "/auto/v"):
			w.Header().Set("Content-Type", "video/mp2t")
			for i := 0; i < 20 && r.Context().Err() == nil; i++ {
				if _, err := w.Write(data); err != nil {
					return
				}
				w.(http.Flusher).Flush()
				time.Sleep(300 * time.Millisecond)
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, srv.URL)
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
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	wctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if _, err := h.Watch(wctx, idOf(t, st, "1"), Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatalf("watch: %v", err)
	}
	// One tuner listed is one stream at a time.
	var busy *BusyError
	if _, err := h.Watch(wctx, idOf(t, st, "2"), Rendition{Video: "copy", Audio: "aac2"}, false); !errors.As(err, &busy) {
		t.Fatalf("second stream on a one-tuner emulator: %v", err)
	}
}
