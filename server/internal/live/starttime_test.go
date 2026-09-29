package live

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// A new tune names every step to its first segment; a watch that joins a
// running picture has nothing to report.
func TestStartTimesNameEachStepOfANewTune(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/sample.ts"
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	srv := &fake.Server{TS: sample, Channels: []fake.Channel{{Number: "5.1", Name: "WTST", Freq: 533000000}}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
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
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	defer h.Shutdown()
	copied := Rendition{Video: "copy", Audio: "copy"}
	id := idOf(t, st, "5.1")

	asked := time.Now()
	session, err := h.Watch(ctx, id, copied, false)
	if err != nil {
		t.Fatal(err)
	}
	var steps string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		steps = h.StartTimes(id, session.Rendition, asked)
		if !strings.Contains(steps, " -") {
			break
		}
	}
	for _, name := range []string{"status ", "lock ", "first byte ", "encode ", "input ", "output ", "part ", "segment "} {
		if !strings.Contains(steps, name) || strings.Contains(steps, name+"-") {
			t.Fatalf("step %q missing from %q", name, steps)
		}
	}

	if joined := h.StartTimes(id, session.Rendition, time.Now()); joined != "" {
		t.Fatalf("a running picture reported %q", joined)
	}
}
