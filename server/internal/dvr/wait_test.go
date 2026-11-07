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
	// One refusal can be a guide scan being stopped for the recording.
	Tick(ctx, st, hub)
	if events := recordingEvents(t, st); len(events) != 0 {
		t.Fatalf("after one refusal: %q", events)
	}
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

// The show is over and only its padding is left: no piece of nothing, and the
// failure waits for the end of the padding.
func TestAWaitDoesNotStartInThePaddingAfterTheShow(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	end := time.Now().Add(-30 * time.Second).Truncate(time.Second)
	start := end.Add(-30 * time.Minute)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: "News", Start: start, End: end}}); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end.Add(2 * time.Minute), PassID: id, Title: "News",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start,
	}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	if recs, _ := st.Recordings(ctx); len(recs) != 0 {
		t.Fatalf("recordings %+v", recs)
	}
	if waits, _ := st.RecordingWaits(ctx); len(waits) != 1 {
		t.Fatalf("waits %+v", waits)
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

func lateWait(t *testing.T, st *store.Store, channel int64, title string, ago time.Duration) store.RecordingWait {
	t.Helper()
	ctx := context.Background()
	start := time.Now().Add(-ago).Truncate(time.Second)
	end := start.Add(time.Hour)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: title, Start: start, End: end}}); err != nil {
		t.Fatal(err)
	}
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: title, Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	w := store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: title,
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
	}
	if err := st.PutRecordingWait(ctx, w); err != nil {
		t.Fatal(err)
	}
	return w
}

// A viewer who got a tuner the retry had waited for keeps it. Nothing warned
// them: the warning before a recording only runs up to its start.
func TestALateRetryLeavesAViewerAlone(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "5.1", Name: "KTWO", Freq: 605000000},
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
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 2 {
		t.Fatalf("%+v %v", chs, err)
	}
	srv.Hold(1)
	if _, err := hub.Watch(ctx, chs[1].ID, live.Rendition{Video: "copy", Audio: "copy"}, false); err != nil {
		t.Fatal(err)
	}
	lateWait(t, st, chs[0].ID, "News", 5*time.Minute)
	Tick(ctx, st, hub)
	if !hub.Watching() {
		t.Fatal("the retry stopped the viewer")
	}
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 0 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	for _, ev := range recordingEvents(t, st) {
		if strings.Contains(ev, "Watching") || strings.Contains(ev, "stop") {
			t.Fatalf("event %q", ev)
		}
	}
}

// Past the start window a retry waits its turn. A guide scan keeps its tuner.
func TestALateRetryLeavesAScanAlone(t *testing.T) {
	srv, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	srv.HoldAll()
	lateWait(t, st, channel, "News", 5*time.Minute)
	scanning := make(chan struct{})
	ended := make(chan error, 1)
	stop, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.RunScan(stop, []int{593000000}, time.Minute, func(ctx context.Context, _ int) error {
		close(scanning)
		<-ctx.Done()
		ended <- ctx.Err()
		return ctx.Err()
	})
	<-scanning
	Tick(ctx, st, hub)
	select {
	case <-ended:
		t.Fatal("the retry stopped the scan")
	case <-time.After(200 * time.Millisecond):
	}
}

// Inside the start window a recording still takes its tuner back. The viewer was warned.
func TestAStartInItsWindowStillStopsAScan(t *testing.T) {
	srv, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	srv.HoldAll()
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: "News", Start: start, End: start.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"}); err != nil {
		t.Fatal(err)
	}
	scanning := make(chan struct{})
	ended := make(chan error, 1)
	stop, cancel := context.WithCancel(ctx)
	defer cancel()
	go hub.RunScan(stop, []int{593000000}, time.Minute, func(ctx context.Context, _ int) error {
		close(scanning)
		<-ctx.Done()
		ended <- ctx.Err()
		return ctx.Err()
	})
	<-scanning
	Tick(ctx, st, hub)
	select {
	case <-ended:
	case <-time.After(2 * time.Second):
		t.Fatal("a recording in its window left the scan running")
	}
}

func TestTunersToHoldCountsFrequencies(t *testing.T) {
	soon := map[string]bool{"box:593": true, "box:605": true}
	if n := tunersToHold(soon, nil); n != 2 {
		t.Fatalf("two frequencies: %d", n)
	}
	if n := tunersToHold(soon, map[string]bool{"box:593": true}); n != 1 {
		t.Fatalf("one already recording: %d", n)
	}
}

// Two channels on one frequency, about to record, need one tuner. The hold used
// to count airings, keep both free tuners back, and refuse a viewer.
func TestHoldingForOneFrequencyLeavesAViewerATuner(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg is not installed")
	}
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "4.2", Name: "KBW2", Freq: 593000000},
		{Number: "5.1", Name: "KTWO", Freq: 605000000},
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
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 3 {
		t.Fatalf("%+v %v", chs, err)
	}
	id := map[string]int64{}
	for _, ch := range chs {
		id[ch.GuideNumber] = ch.ID
	}
	for number, freq := range map[string]int{"4.1": 593000000, "4.2": 593000000, "5.1": 605000000} {
		if err := st.RememberProgram(ctx, "FAKEHDHR", number, freq, 1); err != nil {
			t.Fatal(err)
		}
	}
	// Inside the tick's look-ahead, past the start window's first second.
	start := time.Now().Add(100 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{
		{ChannelID: id["4.1"], Title: "One", Start: start, End: start.Add(time.Hour)},
		{ChannelID: id["4.2"], Title: "Two", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	for _, title := range []string{"One", "Two"} {
		if _, err := st.AddSeriesPass(ctx, store.Pass{Title: title, Kind: "series", MatchKind: "title"}); err != nil {
			t.Fatal(err)
		}
	}
	Tick(ctx, st, hub)
	if recs, _ := st.Recordings(ctx); len(recs) != 0 {
		t.Fatalf("recordings %+v", recs)
	}
	if _, err := hub.Watch(ctx, id["5.1"], live.Rendition{Video: "copy", Audio: "copy"}, false); err != nil {
		t.Fatalf("a viewer was refused with a tuner to spare: %v", err)
	}
}

// Last tick's hold is for this recording. It used to be refused by it, and start
// one tick late.
func TestARecordingIsNotRefusedByItsOwnHold(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: channel, Title: "News", Start: start, End: start.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"}); err != nil {
		t.Fatal(err)
	}
	hub.SetHold(2)
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
}

func waitFor(t *testing.T, st *store.Store, channel int64) (store.RecordingWait, bool) {
	t.Helper()
	waits, err := st.RecordingWaits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range waits {
		if w.ChannelID == channel {
			return w, true
		}
	}
	return store.RecordingWait{}, false
}

// Losing the tuners to a rule above, or the guide moving the show, is not the
// viewer's choice. The airing still ends with a failed recording that says why.
func TestAWaitThatLosesAConflictLeavesATrace(t *testing.T) {
	srv := &fake.Server{Realtime: true, Channels: []fake.Channel{
		{Number: "4.1", Name: "KBWV", Freq: 593000000},
		{Number: "5.1", Name: "KTWO", Freq: 605000000},
		{Number: "6.1", Name: "KSIX", Freq: 617000000},
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
	if err := st.PutSettings(ctx, map[string]string{"watermarkGB": "0"}); err != nil {
		t.Fatal(err)
	}
	hub := live.New(st, t.TempDir(), "ffmpeg", "libx264")
	t.Cleanup(hub.Shutdown)
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 3 {
		t.Fatalf("%+v %v", chs, err)
	}
	freq := map[string]int{"4.1": 593000000, "5.1": 605000000, "6.1": 617000000}
	start := time.Now().Add(30 * time.Second).Truncate(time.Second)
	end := start.Add(time.Hour)
	var last int64
	for i, ch := range chs {
		if err := st.RememberProgram(ctx, "FAKEHDHR", ch.GuideNumber, freq[ch.GuideNumber], 1); err != nil {
			t.Fatal(err)
		}
		title := ch.GuideNumber + " show"
		if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: ch.ID, Title: title, Start: start, End: end}}); err != nil {
			t.Fatal(err)
		}
		id, err := st.AddSeriesPass(ctx, store.Pass{Title: title, Kind: "series", MatchKind: "title"})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdatePass(ctx, id, 0, 0, 5-i); err != nil {
			t.Fatal(err)
		}
		last = ch.ID
		if i == 2 {
			if err := st.PutRecordingWait(ctx, store.RecordingWait{
				ChannelID: ch.ID, Start: start, End: end, Until: end, PassID: id, Title: title,
				Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
			}); err != nil {
				t.Fatal(err)
			}
		}
	}
	srv.HoldAll()
	Tick(ctx, st, hub)
	w, ok := waitFor(t, st, last)
	if !ok {
		t.Fatal("the wait went with no trace")
	}
	if w.Failure != failedConflict {
		t.Fatalf("failure %q", w.Failure)
	}
}

func TestAWaitWhoseListingMovedLeavesATrace(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	start := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Minute)
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "News", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	w := store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: "News",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
	}
	if err := st.PutRecordingWait(ctx, w); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	got, ok := waitFor(t, st, channel)
	if !ok {
		t.Fatal("the wait went with no trace")
	}
	if got.Failure != failedMoved {
		t.Fatalf("failure %q", got.Failure)
	}
	// Its time passes: the failed recording says what happened.
	if err := st.DeleteRecordingWait(ctx, channel, start); err != nil {
		t.Fatal(err)
	}
	got.End, got.Until = time.Now().Add(-time.Minute), time.Now().Add(-time.Minute)
	if err := st.PutRecordingWait(ctx, got); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 || recs[0].Status != "failed" || recs[0].Error != failedMoved {
		t.Fatalf("recordings %+v %v", recs, err)
	}
}

// A game can run hours past its listing. A tuner that comes free for the end of
// it still records the end.
func TestAWaitingGameKeepsTryingPastItsListing(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	on := true
	hub.GameOn = func(_ context.Context, id string) bool { return id == "401" && on }
	end := time.Now().Add(-20 * time.Minute).Truncate(time.Second)
	start := end.Add(-3 * time.Hour)
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "Game", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: "Game", GameID: "401",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
	}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 || recs[0].Status != "recording" || !recs[0].Partial || recs[0].GameID != "401" {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
	if _, waiting := waitFor(t, st, channel); waiting {
		t.Fatal("the wait stayed after it started")
	}
}

func TestAWaitingGameThatEndedSaysSo(t *testing.T) {
	srv, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	on := true
	hub.GameOn = func(context.Context, string) bool { return on }
	end := time.Now().Add(-20 * time.Minute).Truncate(time.Second)
	start := end.Add(-3 * time.Hour)
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "Game", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end, PassID: id, Title: "Game", GameID: "401",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
	}); err != nil {
		t.Fatal(err)
	}
	srv.HoldAll()
	Tick(ctx, st, hub)
	if _, waiting := waitFor(t, st, channel); !waiting {
		t.Fatal("the game is on and the wait ended")
	}
	if recs, _ := st.Recordings(ctx); len(recs) != 0 {
		t.Fatalf("recordings %+v", recs)
	}
	on = false
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 || recs[0].Status != "failed" || recs[0].Error != "Every tuner was busy until the game ended." {
		t.Fatalf("recordings %+v %v", recs, err)
	}
}

// A sports listing with no game to follow has its extra time.
func TestAWaitingSportsShowUsesItsTail(t *testing.T) {
	_, st, hub, channel := waitFixture(t)
	ctx := context.Background()
	end := time.Now().Add(-5 * time.Minute).Truncate(time.Second)
	start := end.Add(-2 * time.Hour)
	id, err := st.AddSeriesPass(ctx, store.Pass{Title: "Football", Kind: "series", MatchKind: "title"})
	if err != nil {
		t.Fatal(err)
	}
	air := store.Airing{Title: "Football", Category: "Sports", Start: start, End: end}
	if SportsTail(air) == 0 {
		t.Fatal("fixture has no tail")
	}
	if err := st.PutRecordingWait(ctx, store.RecordingWait{
		ChannelID: channel, Start: start, End: end, Until: end.Add(SportsTail(air)), PassID: id, Title: "Football", Category: "Sports",
		Reason: "Waiting for a tuner", Failure: "Every tuner was busy.", Since: start, Told: true,
	}); err != nil {
		t.Fatal(err)
	}
	Tick(ctx, st, hub)
	recs, err := st.Recordings(ctx)
	if err != nil || len(recs) != 1 || recs[0].Status != "recording" {
		t.Fatalf("recordings %+v %v", recs, err)
	}
	t.Cleanup(func() { hub.StopRecord(recs[0].ID) })
}
