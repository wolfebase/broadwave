package live

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
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

// emulatorSample is a short TS the fake emulators stream on a loop.
func emulatorSample(t *testing.T) []byte {
	t.Helper()
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
	return data
}

// fakeEmulator answers discover.json and lineup.json and streams /auto/vN
// after answer, as Threadfin does. It has no status.json and no control port.
func fakeEmulator(t *testing.T, st *store.Store, id string, tuners int, numbers []string, answer time.Duration, data []byte) {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/discover.json":
			fmt.Fprintf(w, `{"FriendlyName":"Threadfin","ModelNumber":"HDTC-2US","FirmwareName":"hdhomeruntc_atsc","DeviceID":"%s","BaseURL":"%s","LineupURL":"%s/lineup.json","TunerCount":%d}`, id, srv.URL, srv.URL, tuners)
		case r.URL.Path == "/lineup.json":
			var rows []string
			for _, n := range numbers {
				rows = append(rows, fmt.Sprintf(`{"GuideNumber":"%s","GuideName":"Ch %s","URL":"%s/auto/v%s"}`, n, n, srv.URL, n))
			}
			_, _ = io.WriteString(w, "["+strings.Join(rows, ",")+"]")
		case strings.HasPrefix(r.URL.Path, "/auto/v"):
			select {
			case <-time.After(answer):
			case <-r.Context().Done():
				return
			}
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
}

func emulatorHub(t *testing.T) (*Hub, *store.Store) {
	t.Helper()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	return h, st
}

// Threadfin, xTeVe, Antennas, ErsatzTV, and Dispatcharr answer discover.json
// and lineup.json and stream /auto/v<number>. None has status.json or the
// control port. A watch still plays.
func TestAnHDHomeRunEmulatorPlays(t *testing.T) {
	data := emulatorSample(t)
	h, st := emulatorHub(t)
	fakeEmulator(t, st, "EMU00001", 1, []string{"1", "2"}, 0, data)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if _, err := h.Watch(ctx, idOf(t, st, "1"), Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatalf("watch: %v", err)
	}
	// One tuner listed is one stream at a time.
	var busy *BusyError
	if _, err := h.Watch(ctx, idOf(t, st, "2"), Rendition{Video: "copy", Audio: "aac2"}, false); !errors.As(err, &busy) {
		t.Fatalf("second stream on a one-tuner emulator: %v", err)
	}
	full, err := st.SourceChannel(ctx, idOf(t, st, "1"))
	if err != nil {
		t.Fatal(err)
	}
	if !h.NoStatus(ctx, full.BaseURL) {
		t.Fatal("the emulator is not known to have no status")
	}
	h.mu.Lock()
	m := muxOf(h, h.channels[full.ID])
	reopen := m != nil && m.reopen == full.StreamURL
	h.mu.Unlock()
	if !reopen {
		t.Fatal("an emulator stream has nothing to reopen after a stall")
	}
}

// Threadfin and Dispatcharr can take seconds to send a stream's first byte.
// That wait must not hold the hub: another watch starts meanwhile.
func TestASlowEmulatorDoesNotHoldOtherWatches(t *testing.T) {
	data := emulatorSample(t)
	h, st := emulatorHub(t)
	fakeEmulator(t, st, "EMUSLOW1", 2, []string{"1"}, 4*time.Second, data)
	fakeEmulator(t, st, "EMUFAST1", 2, []string{"7"}, 0, data)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	slow := make(chan error, 1)
	go func() {
		_, err := h.Watch(ctx, idOf(t, st, "1"), Rendition{Video: "copy", Audio: "aac2"}, false)
		slow <- err
	}()
	time.Sleep(500 * time.Millisecond)
	began := time.Now()
	if _, err := h.Watch(ctx, idOf(t, st, "7"), Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatalf("fast watch: %v", err)
	}
	if took := time.Since(began); took > 3*time.Second {
		t.Fatalf("the fast watch waited %v behind the slow emulator", took)
	}
	if err := <-slow; err != nil {
		t.Fatalf("slow watch: %v", err)
	}
}

// A status read cut off mid-body is a slow tuner, not one with no status.
// Marked plain, a real HDHomeRun would tune through an unpooled stream.
func TestATruncatedStatusIsNotAnEmulator(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			buf := make([]byte, 1024)
			_, _ = c.Read(buf)
			_, _ = io.WriteString(c, "HTTP/1.1 200 OK\r\nContent-Type: application/json\r\nContent-Length: 400\r\n\r\n[{\"Resource\":\"tuner0\",")
			c.Close()
		}
	}()
	_, err = fetchTunerStatus(context.Background(), "http://"+ln.Addr().String())
	if err == nil || errors.Is(err, errNoStatus) {
		t.Fatalf("truncated body: %v", err)
	}
	for body, plain := range map[string]bool{"<html>Threadfin</html>": true, `{"error":"no"}`: true, `[{"Resource":`: false} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) }))
		_, err := fetchTunerStatus(context.Background(), srv.URL)
		srv.Close()
		if errors.Is(err, errNoStatus) != plain {
			t.Errorf("%q: %v", body, err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "busy", http.StatusServiceUnavailable) }))
	defer srv.Close()
	if _, err := fetchTunerStatus(context.Background(), srv.URL); err == nil || errors.Is(err, errNoStatus) {
		t.Fatalf("503: %v", err)
	}
	// A device that has answered once stays a tuner.
	h := &Hub{}
	h.noteStatusLocked("http://tuner", nil)
	h.noteStatusLocked("http://tuner", errNoStatus)
	if h.plain["http://tuner"] {
		t.Fatal("a tuner that answered was marked plain")
	}
}

// Diagnostics shows what this server has open on an emulator: every channel
// tuned from it, and one still opening.
func TestPlainStreamsCountsWhatIsOpen(t *testing.T) {
	h, _ := testHub(t)
	const base = "http://emu.invalid:5004"
	h.mu.Lock()
	h.plain = map[string]bool{base: true}
	h.channels[1] = &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 1, DeviceID: "EMU"}}}
	h.channels[2] = &feed{channel: store.SourceChannel{Channel: store.Channel{ID: 2, DeviceID: "TUNER"}}}
	h.plainOpening = map[string]int{"EMU": 1}
	h.mu.Unlock()
	if plain, open := h.PlainStreams(context.Background(), base, "EMU"); !plain || open != 2 {
		t.Fatalf("plain %v open %d", plain, open)
	}
}
