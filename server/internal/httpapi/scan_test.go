package httpapi

import (
	"context"
	"testing"
	"time"

	"broadwave/internal/hdhr"
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
	if ch, ok := srv.nextUnlisted(ctx, map[int]bool{}, map[int64]bool{}); ok {
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
	ch, ok := srv.nextUnlisted(ctx, map[int]bool{}, map[int64]bool{})
	if !ok || ch.GuideName != "Harbor" {
		t.Fatalf("got %+v ok %v", ch, ok)
	}
}
