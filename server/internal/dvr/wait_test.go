package dvr

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

func waitFixture(t *testing.T) (*fake.Server, *store.Store, *live.Hub, int64) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{{Number: "4.1", Name: "KBWV", Freq: 593000000}}}
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
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 {
		t.Fatalf("%+v %v", chs, err)
	}
	return srv, st, hub, chs[0].ID
}

func recordingEvents(t *testing.T, st *store.Store) []string {
	t.Helper()
	events, err := st.Events(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, ev := range events {
		if ev.Kind == "recording" {
			out = append(out, ev.Message)
		}
	}
	return out
}

// Every tuner held by something else: the first refusal says so once, however
// many ticks retry it, and the plan line says it is waiting.
func TestARefusedRecordingSaysItIsWaitingOnce(t *testing.T) {
	srv, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: "News", Start: start, End: start.Add(30 * time.Minute)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"}); err != nil {
		t.Fatal(err)
	}
	srv.HoldAll()
	for range 3 {
		Tick(ctx, st, hub)
	}
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 0 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	events := recordingEvents(t, st)
	if len(events) != 1 || !strings.Contains(events[0], "News is waiting for a tuner") {
		t.Fatalf("events %q", events)
	}
	waits, err := st.RecordingWaits(ctx)
	if err != nil || len(waits) != 1 {
		t.Fatalf("waits %+v %v", waits, err)
	}
	items := []Planned{{Airing: store.Airing{ChannelID: channel, Start: start}}, {Airing: store.Airing{ChannelID: channel, Start: start.Add(time.Hour)}}}
	MarkWaits(items, waits)
	if !items[0].Waiting || items[0].Reason != "Waiting for a tuner" {
		t.Fatalf("plan line %+v", items[0])
	}
	if items[1].Waiting {
		t.Fatalf("another airing is not waiting: %+v", items[1])
	}

	// A tuner comes free: the retry starts it, and the wait is over.
	srv.FreeAll()
	Tick(ctx, st, hub)
	recs, err = st.Recordings(ctx)
	if err != nil || len(recs) != 1 {
		t.Fatalf("after a tuner came free: %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
	waits, _ = st.RecordingWaits(ctx)
	if len(waits) != 0 {
		t.Fatalf("waits after the start: %+v", waits)
	}
}

// The retry used to end 90 s after the start, so a tuner that came free later
// recorded nothing. A late start says what it missed and is not the episode.
func TestAWaitingRecordingStartsLateWhenATunerComesFree(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(-10 * time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Minute)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: "News", Start: start, End: end}}); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: "News",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start,
	}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
	if !recs[0].Partial {
		t.Fatalf("a recording that missed its start is partial: %+v", recs[0])
	}
	events := strings.Join(recordingEvents(t, st), "\n")
	if !strings.Contains(events, "News started 10 min late") {
		t.Fatalf("events %q", events)
	}
}

// The whole airing passes with no tuner. It used to be lost without a trace.
func TestAnAiringThatNeverStartedLeavesAFailedRecording(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(-40 * time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Minute)
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: "News", Subtitle: "Late edition",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start,
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		Tick(ctx, st, hub)
	}
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	got := recs[0]
	if got.Status != "failed" || got.Error != "Every tuner was busy." || got.Title != "News" || got.Subtitle != "Late edition" || got.PassID != id || !got.StartedAt.Equal(start) {
		t.Fatalf("failed row %+v", got)
	}
	if got.GuideNumber != "4.1" {
		t.Fatalf("guide number %q", got.GuideNumber)
	}
	events := recordingEvents(t, st)
	if len(events) != 1 || !strings.Contains(events[0], "News did not record. Every tuner was busy.") {
		t.Fatalf("events %q", events)
	}
	if waits, _ := st.RecordingWaits(ctx); len(waits) != 0 {
		t.Fatalf("waits %+v", waits)
	}
	// It has no file, and removing it from Recordings touches nothing else.
	if err := hub.RemoveRecordingFiles(got); err != nil {
		t.Fatal(err)
	}
	// The failed row is not the episode: the next airing still records.
	if haveEpisode(store.Pass{}, recs, nil, store.EpisodeKey("", "News", "Late edition", channel)) {
		t.Fatal("a recording that never started counts as having the episode")
	}
}

// A rule removed, or the airing skipped, while it waited is not a failure.
func TestAWaitTheViewerCalledOffLeavesNoFailure(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(-40 * time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Minute)
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: 999, Title: "News", Failure: "Every tuner was busy.", Since: start,
	}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, _ := st.Recordings(ctx)
	if len(recs) != 0 {
		t.Fatalf("recordings %+v", recs)
	}
	if waits, _ := st.RecordingWaits(ctx); len(waits) != 0 {
		t.Fatalf("waits %+v", waits)
	}

	// Skipped once while it is on.
	onNow := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	onEnd := onNow.Add(30 * time.Minute)
	air := store.Airing{ChannelID: channel, Title: "Weather", Start: onNow, End: onEnd}
	if err := st.InsertAirings(ctx, []store.Airing{air}); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "Weather", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: onNow, End: onEnd, Until: onEnd, PassID: id, Title: "Weather", Failure: "Every tuner was busy.", Since: onNow,
	}); err != nil {
		t.Fatal(err)
	}
	key, starts := SkipParts(air)
	if err := st.SkipAiring(ctx, key, starts); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	if waits, _ := st.RecordingWaits(ctx); len(waits) != 0 {
		t.Fatalf("waits after a skip %+v", waits)
	}
	if recs, _ := st.Recordings(ctx); len(recs) != 0 {
		t.Fatalf("recordings after a skip %+v", recs)
	}
}

// The disk's refusal note is written by every attempt, and an attempt runs
// every 20 s while the airing is on.
func TestARepeatedDiskRefusalIsOneEvent(t *testing.T) {
	st, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	for range 3 {
		if err := st.AddEvent(ctx, "disk", "Refused a recording because free space is under the reserve"); err != nil {
			t.Fatal(err)
		}
	}
	events, err := st.Events(ctx, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("events %+v %v", events, err)
	}
}
