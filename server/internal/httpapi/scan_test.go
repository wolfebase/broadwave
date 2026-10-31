package httpapi

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/hdhr/fake"
	"broadwave/internal/live"
	"broadwave/internal/store"
)

// A listing that starts within the next minute already counts as a guide,
// so a scan does not tune that channel when its frequency is unknown.
func TestNextUnlistedSkipsAShowThatIsAboutToStart(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "Duo", BaseURL: "http://127.0.0.1", TunerCount: 2}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "Harbor"},
		{GuideNumber: "4.2", GuideName: "Cedar"},
	}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	var rows []store.Airing
	for _, ch := range channels {
		rows = append(rows, store.Airing{
			ChannelID: ch.ID, Title: "Quiz Night",
			Start: now.Add(30 * time.Second), End: now.Add(30 * time.Minute),
		})
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st}
	if ch, _, ok := srv.nextUnlisted(ctx, map[int]bool{}, map[int64]bool{}); ok {
		t.Fatalf("scanned %s; a show starting within a minute is enough of a guide", ch.GuideName)
	}
}

func TestNextUnlistedTunesAChannelWithOnlyTheCurrentShow(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "Duo", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "Harbor"},
	}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatal(err, channels)
	}
	now := time.Now()
	if err := st.InsertAirings(ctx, []store.Airing{{
		ChannelID: channels[0].ID, Title: "News",
		Start: now.Add(-20 * time.Minute), End: now.Add(10 * time.Minute),
	}}); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st}
	ch, _, ok := srv.nextUnlisted(ctx, map[int]bool{}, map[int64]bool{})
	if !ok || ch.GuideName != "Harbor" {
		t.Fatalf("got %+v ok %v", ch, ok)
	}
}

// A station off the air is tuned once by the guide scan, not once per
// channel it carries: its other channels share the frequency.
func TestTheGuideScanTunesADarkStationOnce(t *testing.T) {
	ctx := context.Background()
	sample := filepath.Join(t.TempDir(), "sample.ts")
	contractSample(t, sample)
	tuner := &fake.Server{TS: sample}
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
	// 4.1 and 4.2 are one station, as a channel scan records them: the
	// frequency is known, so its other channels can be skipped.
	for _, guide := range []string{"4.1", "4.2"} {
		if err := st.RememberProgram(ctx, dev.DeviceID, guide, 593000000, 0); err != nil {
			t.Fatal(err)
		}
	}
	tuner.Dark("4.1")
	tuner.Dark("4.2")
	was := scanDwell
	scanDwell = time.Second
	t.Cleanup(func() { scanDwell = was })
	hub := &live.Hub{Store: st, Dir: t.TempDir(), Encoder: "libx264", FFmpeg: "ffmpeg"}
	t.Cleanup(hub.Shutdown)
	srv := &Server{Store: st, HDHR: client, Hub: hub}

	srv.scanMissing(ctx)

	// Each tune sets a tuner's virtual channel once: the dark 4.1, then 5.1.
	// Without the skip, 4.2 was tuned too and found the same nothing.
	tunes := 0
	for _, req := range tuner.Requests() {
		if strings.HasSuffix(req, "/vchannel") {
			tunes++
		}
	}
	if tunes != 2 {
		t.Fatalf("the scan tuned %d times, want 2 (4.1 dark, then 5.1): %v", tunes, tuner.Requests())
	}
	if !slices.ContainsFunc(tuner.Requests(), func(r string) bool { return strings.HasSuffix(r, "/ch533000000") }) {
		t.Fatalf("the station on the air was not scanned: %v", tuner.Requests())
	}
}

// ATSC listings come only from an antenna tuner. The scan must not hold a
// playlist stream or a DVB or cable tuner for a guide that never comes.
func TestNextUnlistedSkipsPlaylistsDVBAndCable(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, dev := range []hdhr.Device{
		{DeviceID: "DVB", FriendlyName: "Quatro", ModelNumber: "HDHR5-4DT", BaseURL: "http://127.0.0.1:1", TunerCount: 4},
		{DeviceID: "CC", FriendlyName: "Prime", ModelNumber: "HDHR3-CC", BaseURL: "http://127.0.0.1:2", TunerCount: 3},
		{DeviceID: "src-1", FriendlyName: "IPTV", ModelNumber: "m3u", BaseURL: "source"},
	} {
		if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{{GuideNumber: "5", GuideName: dev.FriendlyName, StreamURL: "http://127.0.0.1/5"}}); err != nil {
			t.Fatal(err)
		}
	}
	srv := &Server{Store: st}
	if ch, _, ok := srv.nextUnlisted(ctx, map[int]bool{}, map[int64]bool{}); ok {
		t.Fatalf("scanned %s", ch.GuideName)
	}
}
