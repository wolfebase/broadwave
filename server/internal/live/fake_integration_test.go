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

func TestFakeTunerSharesFrequencyAndYields(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	sample := t.TempDir() + "/sample.ts"
	cmd := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=30",
		"-f", "lavfi", "-i", "sine=frequency=440",
		"-t", "1", "-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-f", "mpegts", sample)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sample: %v %s", err, out)
	}
	srv := &fake.Server{TS: sample, Channels: []fake.Channel{
		{Number: "4.1", Name: "WDAF", Freq: 593000000},
		{Number: "4.2", Name: "WDAF2", Freq: 593000000},
		{Number: "5.1", Name: "KCTV", Freq: 533000000},
		{Number: "9.1", Name: "KMBC", Freq: 563000000},
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
	h := New(st, t.TempDir(), "ffmpeg", "libx264")
	defer func() {
		for _, rec := range mustRecordings(t, st) {
			if rec.Status == "recording" {
				h.StopRecord(rec.ID)
			}
		}
	}()

	a := idOf(t, st, "4.1")
	b := idOf(t, st, "4.2")
	c := idOf(t, st, "5.1")
	d := idOf(t, st, "9.1")

	started := make(chan struct{})
	done := make(chan struct{})
	go func() {
		h.RunScan(context.Background(), []int{1}, time.Minute, func(ctx context.Context, _ int) error {
			close(started)
			_, err := h.Measure(ctx, a)
			return err
		})
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("scan did not start")
	}
	rec, err := h.Record(ctx, a, 2, "News")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("a recording did not stop the scan")
	}
	if _, err := h.Record(ctx, b, 2, "News 2"); err != nil {
		t.Fatal(err)
	}
	tuners, err := h.Tuners(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ours := 0
	for _, tuner := range tuners {
		if tuner.Ours {
			ours++
		}
	}
	if ours != 1 {
		t.Fatalf("4.1 and 4.2 should share one tuner, got %d ours in %+v", ours, tuners)
	}
	full, err := st.SourceChannel(ctx, a)
	if err != nil || full.FrequencyHz != 593000000 {
		t.Fatalf("frequency %+v %v", full.FrequencyHz, err)
	}
	plan := PlanMultiview([]PlanChannel{
		{ID: a, FrequencyHz: 593000000, Number: "4.1"},
		{ID: b, FrequencyHz: 593000000, Number: "4.2"},
		{ID: c, FrequencyHz: 533000000, Number: "5.1"},
	}, 2, nil, nil)
	if len(plan.Blocked) != 0 {
		t.Fatalf("two frequencies fit on two tuners: %+v", plan.Blocked)
	}
	if _, err := h.Record(ctx, c, 2, "Other"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Record(ctx, d, 2, "Third"); err == nil {
		t.Fatal("expected both tuners to be busy")
	}
	until := time.Now().Add(5 * time.Minute)
	if err := h.ExtendRecording(ctx, rec.ID, until); err != nil {
		t.Fatal(err)
	}
	got, err := st.Recording(ctx, rec.ID)
	if err != nil || got.EndsAt == nil || got.EndsAt.Before(time.Now().Add(4*time.Minute)) {
		t.Fatalf("extend %+v %v", got.EndsAt, err)
	}
	// Stop only once ffmpeg has written the file: with a second of the fake
	// stream it cannot identify the streams yet and exits without output.
	var info os.FileInfo
	for deadline := time.Now().Add(20 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		if info, err = os.Stat(got.Path); err == nil && info.Size() > 0 {
			break
		}
	}
	if err != nil || info.Size() == 0 {
		t.Fatalf("recording file %v", err)
	}
	h.StopRecord(rec.ID)
	if after, err := st.Recording(ctx, rec.ID); err != nil || after.Status != "complete" {
		t.Fatalf("stopped recording %+v %v", after.Status, err)
	}

	for _, row := range mustRecordings(t, st) {
		if row.Status == "recording" {
			h.StopRecord(row.ID)
		}
	}
	time.Sleep(500 * time.Millisecond)
	future := time.Now().Add(10 * time.Minute)
	cut, err := st.CreateRecording(ctx, store.Recording{
		ChannelID: c, GuideNumber: "5.1", Title: "Still on", Path: sample, Status: "recording", StartedAt: time.Now(), EndsAt: &future,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.FinishRecording(ctx, cut, "failed", "Stopped when the server restarted."); err != nil {
		t.Fatal(err)
	}
	left, err := st.Recording(ctx, cut)
	if err != nil || left.Status != "failed" {
		t.Fatalf("restart should close the cut-off recording: %+v %v", left.Status, err)
	}
	again, err := h.Record(ctx, c, 2, "Still on")
	if err != nil || again.Status != "recording" {
		t.Fatalf("resume %+v %v", again.Status, err)
	}
}

func idOf(t *testing.T, st *store.Store, number string) int64 {
	t.Helper()
	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, ch := range channels {
		if ch.GuideNumber == number {
			return ch.ID
		}
	}
	t.Fatalf("missing %s", number)
	return 0
}

func mustRecordings(t *testing.T, st *store.Store) []store.Recording {
	t.Helper()
	recs, err := st.Recordings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return recs
}
