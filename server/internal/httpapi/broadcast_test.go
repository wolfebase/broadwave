package httpapi

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/psip"
	"broadwave/internal/store"
)

func TestBroadcastFillsOnlyGaps(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	err := st.UpsertDevice(ctx, hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF", VideoCodec: "MPEG2", AudioCodec: "AC3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatal(err, len(channels))
	}
	start := time.Date(2026, 9, 24, 20, 0, 0, 0, time.UTC)
	if err := st.InsertAirings(ctx, []store.Airing{{
		ChannelID: channels[0].ID, Title: "Jeopardy!", Start: start, End: start.Add(30 * time.Minute),
	}}); err != nil {
		t.Fatal(err)
	}
	n, err := (&Server{Store: st}).ApplyBroadcast(ctx, psip.Guide{
		Channels: []psip.Channel{{Major: 4, Minor: 1, SourceID: 3, ShortName: "WDAF-DT"}},
		Events: []psip.Event{
			{SourceID: 3, EventID: 1, Title: "JEOPARDY!", Start: start, End: start.Add(30 * time.Minute)},
			{SourceID: 3, EventID: 2, Title: "WHEEL OF FORTUNE", Start: start.Add(30 * time.Minute), End: start.Add(time.Hour)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("filled %d", n)
	}
	rows, err := st.Airings(ctx, start.Add(-time.Minute), start.Add(2*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Title != "Jeopardy!" || rows[1].Title != "Wheel Of Fortune" || rows[1].GuideSource != "broadcast" {
		t.Fatalf("%+v", rows)
	}
}
