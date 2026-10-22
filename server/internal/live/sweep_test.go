package live

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/store"
)

// sweepHub is a hub on a fake tuner with three frequencies, and a TS sample
// whose stills a grab can read.
func sweepHub(t *testing.T, tuners int) (*Hub, *store.Store, *fake.Server) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/sample.ts"
	gen := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "2", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-g", "15",
		"-c:a", "aac", "-f", "mpegts", sample)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	srv := &fake.Server{TS: sample, TunerCount: tuners, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
		{Number: "6.1", Name: "KSIX", Freq: 545000000},
	}}
	base, port, err := srv.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Setenv("HDHR_CONTROL_PORT", port)
	ctx := context.Background()
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	client := &hdhr.Client{}
	dev, err := client.FetchDevice(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	lineup, err := client.FetchLineup(ctx, dev.LineupURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, dev, lineup); err != nil {
		t.Fatal(err)
	}
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	return h, st, srv
}

func tunedCount(h *Hub) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.muxes)
}

func hasStill(h *Hub, id int64) bool {
	_, err := os.Stat(FramePath(h.Dir, id, 0))
	return err == nil
}

// Favorites come first, then guide order; one target per frequency; hidden
// and copy-protected channels are left out.
func TestSweepTargetsOrder(t *testing.T) {
	srv := &fake.Server{Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "4.2", Name: "KBWV2", Freq: 593000000},
		{Number: "5.1", Name: "WTST", Freq: 533000000},
		{Number: "6.1", Name: "KSIX", Freq: 545000000},
		{Number: "7.1", Name: "KSVN", Freq: 557000000},
	}}
	st, _ := tuneStore(t, srv)
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	ctx := context.Background()
	// Frequencies a tune learned.
	ch, err := st.SourceChannel(ctx, idOf(t, st, "4.1"))
	if err != nil {
		t.Fatal(err)
	}
	for n, number := range []string{"4.1", "4.2"} {
		if err := st.RememberProgram(ctx, ch.DeviceID, number, 593000000, n+1); err != nil {
			t.Fatal(err)
		}
	}
	yes := true
	if _, err := st.PatchChannel(ctx, idOf(t, st, "6.1"), store.ChannelPatch{Favorite: &yes}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PatchChannel(ctx, idOf(t, st, "7.1"), store.ChannelPatch{Hidden: &yes}); err != nil {
		t.Fatal(err)
	}
	targets := h.sweepTargets(ctx)
	var got []string
	for _, target := range targets {
		got = append(got, target.first.GuideNumber)
	}
	if len(got) != 3 || got[0] != "6.1" || got[1] != "4.1" || got[2] != "5.1" {
		t.Fatalf("order %v", got)
	}
	if len(targets[1].channels) != 2 {
		t.Fatalf("4.1 and 4.2 are one tune: %+v", targets[1])
	}
}

// Every gate shuts the sweep: the setting, a recording soon, a hold, a
// tune on the device, a guide scan, and fewer than two free tuners.
func TestSweepGates(t *testing.T) {
	srv := &fake.Server{Profile: fake.ProfileConnectDuo, TunerCount: 3}
	st, _ := tuneStore(t, srv)
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(h.Shutdown)
	ctx := context.Background()
	ch, err := st.SourceChannel(ctx, idOf(t, st, "5.1"))
	if err != nil {
		t.Fatal(err)
	}
	if !h.sweepAllowed(ctx, ch) {
		t.Fatal("an idle device with three free tuners is shut")
	}
	if err := st.PutSettings(ctx, map[string]string{SettingStillSweep: "0"}); err != nil {
		t.Fatal(err)
	}
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("the setting is off")
	}
	if err := st.PutSettings(ctx, map[string]string{SettingStillSweep: "1"}); err != nil {
		t.Fatal(err)
	}
	h.RecordingSoon = func(context.Context, time.Duration) bool { return true }
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("a recording starts soon")
	}
	h.RecordingSoon = nil
	h.SetHold(1)
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("a tuner is held for a recording")
	}
	h.SetHold(0)
	h.mu.Lock()
	h.scanCancel = func() {}
	h.mu.Unlock()
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("a guide scan is running")
	}
	h.mu.Lock()
	h.scanCancel = nil
	h.mu.Unlock()
	// Another app on two of the three tuners leaves one: never the last.
	srv.Hold(2)
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("took the last free tuner")
	}
	srv.FreeAll()
	if _, err := h.Watch(ctx, idOf(t, st, "4.1"), Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatal(err)
	}
	if h.sweepAllowed(ctx, ch) {
		t.Fatal("someone is watching on this device")
	}
	t.Setenv("BROADWAVE_STILL_SWEEP", "0")
	done := make(chan struct{})
	go func() {
		h.RunStillSweep(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("BROADWAVE_STILL_SWEEP=0 did not stop the sweep")
	}
}

// A pass grabs a still for every channel, one tune at a time, and lets each
// tuner go at once. A swept still is noted, and a dark target is skipped.
func TestSweepPassGrabsEveryChannel(t *testing.T) {
	h, st, _ := sweepHub(t, 3)
	sweepPause, sweepIdle = 50*time.Millisecond, 50*time.Millisecond
	t.Cleanup(func() { sweepPause, sweepIdle = 20*time.Second, 30*time.Second })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	most := 0
	go func() {
		defer close(done)
		h.RunStillSweep(ctx)
	}()
	ids := []int64{idOf(t, st, "4.1"), idOf(t, st, "5.1"), idOf(t, st, "6.1")}
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		most = max(most, tunedCount(h))
		all := true
		for _, id := range ids {
			all = all && hasStill(h, id)
		}
		if all {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	for _, id := range ids {
		if !hasStill(h, id) {
			t.Fatalf("no still for %d", id)
		}
		if _, ok := h.SweptAt(id); !ok {
			t.Fatalf("still for %d not noted as swept", id)
		}
	}
	if most > 1 {
		t.Fatalf("the sweep held %d tuners at once", most)
	}
	waitFor(t, func() bool { return tunedCount(h) == 0 }, "the last tune was not let go")
	// A fresh still is not swept again, and a dark target waits an hour.
	targets := h.sweepTargets(context.Background())
	if h.sweepDue(context.Background(), targets[0]) {
		t.Fatal("a fresh still is due again")
	}
	h.mu.Lock()
	h.sweepDark = map[string]time.Time{targets[1].key: time.Now()}
	h.mu.Unlock()
	if err := os.Remove(FramePath(h.Dir, targets[1].channels[0], 0)); err != nil {
		t.Fatal(err)
	}
	if h.sweepDue(context.Background(), targets[1]) {
		t.Fatal("a dark target is due within the hour")
	}
}

// A watch on another frequency gets the tuner back before it tunes; a watch
// on the swept frequency joins the sweep's tune.
func TestAWatchStopsTheSweep(t *testing.T) {
	h, st, _ := sweepHub(t, 3)
	sweepGrabWait = time.Minute
	t.Cleanup(func() { sweepGrabWait = 30 * time.Second })
	ctx := context.Background()
	targets := h.sweepTargets(ctx)
	var four sweepTarget
	for _, target := range targets {
		if target.first.GuideNumber == "4.1" {
			four = target
		}
	}
	// No still yet, so the sweep waits on the grab while the watch arrives.
	run := func() chan struct{} {
		done := make(chan struct{})
		go func() {
			h.sweepOne(ctx, sweepTarget{key: four.key, first: four.first, channels: append([]int64{999}, four.channels...)})
			close(done)
		}()
		waitFor(t, func() bool {
			h.mu.Lock()
			defer h.mu.Unlock()
			return h.sweep != nil && h.sweep.feed != nil
		}, "the sweep did not tune")
		return done
	}
	done := run()
	other := idOf(t, st, "5.1")
	h.StopSweep(533000000)
	h.mu.Lock()
	gone := h.channels[four.first.ID] == nil
	h.mu.Unlock()
	if !gone {
		t.Fatal("the swept tune was still held when the watch tuned")
	}
	if _, err := h.Watch(ctx, other, Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("the sweep kept waiting after it was stopped")
	}

	done = run()
	h.mu.Lock()
	swept := h.channels[four.first.ID]
	h.mu.Unlock()
	if _, err := h.Watch(ctx, four.first.ID, Rendition{Video: "copy", Audio: "aac2"}, false); err != nil {
		t.Fatal(err)
	}
	<-done
	h.mu.Lock()
	joined := h.channels[four.first.ID] == swept
	h.mu.Unlock()
	if !joined {
		t.Fatal("a watch of the swept channel tuned again instead of joining")
	}

	// A guess at the next channel (a viewer in the guide) stops it too.
	done = run()
	_, _ = h.Warm(ctx, idOf(t, st, "6.1"), Rendition{Video: "copy", Audio: "aac2"}, false)
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a warm did not stop the sweep")
	}
}

func waitFor(t *testing.T, ok func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if ok() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal(msg)
}
