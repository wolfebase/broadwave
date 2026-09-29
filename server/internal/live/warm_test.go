package live

import (
	"context"
	"os/exec"
	"testing"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// A channel left a moment ago keeps its tuner for a quick flip back. It must
// not make the next channel wait for that hold to run out.
func TestAWarmChannelGivesUpItsTuner(t *testing.T) {
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
	srv := &fake.Server{TS: sample, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
		{Number: "9.1", Name: "KRVR", Freq: 563000000},
		{Number: "11.1", Name: "KTWO", Freq: 503000000},
	}}
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
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	defer h.Shutdown()
	copied := Rendition{Video: "copy", Audio: "copy"}

	// One tuner records; the viewer has the other.
	rec, err := h.Record(ctx, idOf(t, st, "4.1"), 2, "News")
	if err != nil {
		t.Fatal(err)
	}
	defer h.StopRecord(rec.ID)
	first := idOf(t, st, "5.1")
	session, err := h.Watch(ctx, first, copied, false)
	if err != nil {
		t.Fatal(err)
	}
	h.Release(first, session.Rendition)

	// The viewer flips to another frequency while 5.1 is still warm.
	next := idOf(t, st, "9.1")
	if _, err := h.Watch(ctx, next, copied, false); err != nil {
		t.Fatalf("the warm channel kept its tuner: %v", err)
	}
	h.mu.Lock()
	_, warm := h.channels[first]
	_, recording := h.channels[idOf(t, st, "4.1")]
	h.mu.Unlock()
	if warm {
		t.Fatal("5.1 should have left with its tuner")
	}
	if !recording {
		t.Fatal("the recording must keep its tuner")
	}

	// A channel someone is watching is never taken.
	if _, err := h.Watch(ctx, idOf(t, st, "11.1"), copied, false); err == nil {
		t.Fatal("a watched channel gave up its tuner")
	}
}
