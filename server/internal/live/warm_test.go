package live

import (
	"context"
	"os/exec"
	"testing"
	"time"

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

// A guess at the next channel starts a picture only on a frequency already
// tuned, never takes a picture from the budget, runs two at a time (channel
// up and channel down), and is the picture the watch that follows joins.
func TestWarmStartsAGuessOnlyWhereItCostsNothing(t *testing.T) {
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
		{Number: "4.2", Name: "KBWV2", Freq: 593000000},
		{Number: "4.3", Name: "KBWV3", Freq: 593000000},
		{Number: "4.4", Name: "KBWV4", Freq: 593000000},
		{Number: "4.5", Name: "KBWV5", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
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
	// The picture policy alone: no tune kept for a return, and no 1.0 tune
	// for a guess (TestAGuessTunesA1Point0Channel).
	h.TunerLinger = 0
	h.Host.Tiles = 1
	tile := Rendition{Video: "360", Audio: "aac2"}
	running := func(number string) *rendition {
		h.mu.Lock()
		defer h.mu.Unlock()
		if f := h.channels[idOf(t, st, number)]; f != nil {
			return f.renditions[tile.normalized().Key()]
		}
		return nil
	}
	tuned := func() int {
		h.mu.Lock()
		defer h.mu.Unlock()
		return len(h.muxes)
	}

	if ok, err := h.Warm(ctx, idOf(t, st, "4.2"), tile, false); err != nil || ok || tuned() != 0 {
		t.Fatalf("a guess tuned: ok %v err %v tuners %d", ok, err, tuned())
	}
	first := idOf(t, st, "4.1")
	session, err := h.Watch(ctx, first, tile, false)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := h.Warm(ctx, idOf(t, st, "4.2"), tile, false); ok || running("4.1") == nil {
		t.Fatal("a guess took the picture someone is watching")
	}
	h.Release(first, session.Rendition)
	if ok, _ := h.Warm(ctx, idOf(t, st, "4.2"), tile, false); ok || running("4.1") == nil {
		t.Fatal("a guess took the picture kept for a flip back")
	}
	if ok, _ := h.Warm(ctx, idOf(t, st, "5.1"), tile, false); ok || tuned() != 1 {
		t.Fatal("a guess on another frequency took a tuner")
	}

	h.mu.Lock()
	h.Host.Tiles = 3
	h.mu.Unlock()
	if ok, err := h.Warm(ctx, idOf(t, st, "4.2"), tile, false); err != nil || !ok {
		t.Fatalf("no guess on a tuned frequency: %v", err)
	}
	guessed := running("4.2")
	if guessed == nil || guessed.viewers != 0 {
		t.Fatal("the guess counts a viewer")
	}
	if _, err := h.Watch(ctx, idOf(t, st, "4.2"), tile, false); err != nil {
		t.Fatal(err)
	}
	if r := running("4.2"); r != guessed || r.guess {
		t.Fatal("the watch did not join the guessed picture")
	}

	h.mu.Lock()
	h.Host.Tiles = 4
	// Long enough that the two guesses outlive the third Warm on a slow,
	// race-instrumented runner; short enough that the idle check below ends.
	h.RenditionIdle = 1500 * time.Millisecond
	h.mu.Unlock()
	if ok, _ := h.Warm(ctx, idOf(t, st, "4.3"), tile, false); !ok {
		t.Fatal("no guess for 4.3")
	}
	if ok, _ := h.Warm(ctx, idOf(t, st, "4.4"), tile, false); !ok {
		t.Fatal("no guess for 4.4")
	}
	if running("4.3") == nil || running("4.4") == nil {
		t.Fatal("channel up and channel down are not both warm")
	}
	if ok, _ := h.Warm(ctx, idOf(t, st, "4.5"), tile, false); !ok {
		t.Fatal("no guess for 4.5")
	}
	if running("4.3") != nil || running("4.4") == nil || running("4.5") == nil {
		t.Fatal("a third guess did not replace the oldest")
	}
	deadline := time.Now().Add(6 * time.Second)
	for (running("4.4") != nil || running("4.5") != nil) && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if running("4.4") != nil || running("4.5") != nil {
		t.Fatal("a guess nobody joined kept running")
	}
	if running("4.2") == nil || tuned() != 1 {
		t.Fatal("the watched channel or its tuner went with the guess")
	}
}

// Channel up and channel down on other frequencies are both tuned ahead,
// while a tuner stays free; a third guess replaces the oldest, and with
// tuners short only the newest is kept.
func TestGuessesTuneBothNeighbours(t *testing.T) {
	chans := []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "4.2", Name: "KBWV2", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
		{Number: "6.1", Name: "KSIX", Freq: 545000000},
		{Number: "7.1", Name: "KSVN", Freq: 557000000},
	}
	tunedIDs := func(h *Hub) map[int64]bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		out := map[int64]bool{}
		for id := range h.channels {
			out[id] = true
		}
		return out
	}
	srv := &fake.Server{Channels: chans, TunerCount: 4}
	st, _ := tuneStore(t, srv)
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	h.TunerLinger = time.Minute
	copyAAC := Rendition{Video: "copy", Audio: "aac2"}
	ctx := context.Background()
	a, b, c := idOf(t, st, "5.1"), idOf(t, st, "6.1"), idOf(t, st, "7.1")
	_, _ = h.Warm(ctx, a, copyAAC, false)
	_, _ = h.Warm(ctx, b, copyAAC, false)
	if got := tunedIDs(h); !got[a] || !got[b] {
		t.Fatalf("up and down are not both tuned: %v", got)
	}
	_, _ = h.Warm(ctx, c, copyAAC, false)
	if got := tunedIDs(h); got[a] || !got[b] || !got[c] {
		t.Fatalf("a third guess did not replace the oldest: %v", got)
	}

	// Three tuners, one watched: a second guess would leave none free.
	srv3 := &fake.Server{Channels: chans, TunerCount: 3}
	st3, _ := tuneStore(t, srv3)
	h3 := New(st3, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h3.Shutdown)
	h3.TunerLinger = time.Minute
	watched := idOf(t, st3, "4.1")
	if _, err := h3.Watch(ctx, watched, copyAAC, false); err != nil {
		t.Fatal(err)
	}
	a3, b3 := idOf(t, st3, "5.1"), idOf(t, st3, "6.1")
	_, _ = h3.Warm(ctx, a3, copyAAC, false)
	if got := tunedIDs(h3); !got[a3] {
		t.Fatalf("no guess with two tuners free: %v", got)
	}
	_, _ = h3.Warm(ctx, b3, copyAAC, false)
	if got := tunedIDs(h3); got[a3] || !got[b3] || !got[watched] {
		t.Fatalf("with one tuner to spare only the newest guess stays: %v", got)
	}
}
