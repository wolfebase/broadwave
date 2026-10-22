package store

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestAiringFeaturesAndChannelReadings(t *testing.T) {
	ctx := context.Background()
	st := openTestStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "D1", BaseURL: "http://127.0.0.1", TunerCount: 2}, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}}); err != nil {
		t.Fatal(err)
	}
	chs, err := st.Channels(ctx, false)
	if err != nil || len(chs) != 1 {
		t.Fatal(err, chs)
	}
	id := chs[0].ID
	if chs[0].Signal != nil || chs[0].Captions || chs[0].Audio != "" {
		t.Fatalf("readings before a tune: %+v", chs[0])
	}
	start := time.Date(2026, 10, 22, 1, 0, 0, 0, time.UTC)
	if err := st.ReplaceAirings(ctx, []Airing{
		{ChannelID: id, Title: "News", Start: start, End: start.Add(time.Hour), CC: true, Video: "hd", Audio: "surround"},
		{ChannelID: id, Title: "Quiet", Start: start.Add(time.Hour), End: start.Add(2 * time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := st.Airings(ctx, start, start.Add(2*time.Hour))
	if err != nil || len(got) != 2 {
		t.Fatal(err, got)
	}
	if !got[0].CC || got[0].Video != "hd" || got[0].Audio != "surround" || got[1].CC || got[1].Video != "" || got[1].Audio != "" {
		t.Fatalf("%+v", got)
	}
	if err := st.SetChannelStreamInfo(ctx, id, true, "stereo"); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveChannelSignal(ctx, id, 91, 87, 100, start); err != nil {
		t.Fatal(err)
	}
	chs, _ = st.Channels(ctx, false)
	if !chs[0].Captions || chs[0].Audio != "stereo" || chs[0].Signal == nil || chs[0].Signal.Quality != 87 || chs[0].Signal.Strength != 91 || !chs[0].Signal.At.Equal(start) {
		t.Fatalf("%+v %+v", chs[0], chs[0].Signal)
	}
	// An empty layout keeps the one stored.
	if err := st.SetChannelStreamInfo(ctx, id, false, ""); err != nil {
		t.Fatal(err)
	}
	chs, _ = st.Channels(ctx, false)
	if chs[0].Captions || chs[0].Audio != "stereo" {
		t.Fatalf("%+v", chs[0])
	}
}
