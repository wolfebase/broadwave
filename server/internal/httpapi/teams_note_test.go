package httpapi

import (
	"context"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestNoteTeamsAnnouncesTheFollowedGame(t *testing.T) {
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
	if err := st.FollowTeam(ctx, store.TeamFollow{Name: "Harbor Wolves", Short: "Wolves", Abbr: "WLV"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	rows := []store.Airing{{
		ChannelID: channels[0].ID, Title: "Wolves at Harbor",
		Start: now.Add(2 * time.Hour), End: now.Add(5 * time.Hour),
	}}
	for i := range 30 {
		at := now.Add(time.Duration(i) * time.Hour)
		rows = append(rows, store.Airing{
			ChannelID: channels[0].ID, Title: "Evening News", Description: "Wolves visit tomorrow",
			Start: at, End: at.Add(30 * time.Minute),
		})
	}
	if err := st.InsertAirings(ctx, rows); err != nil {
		t.Fatal(err)
	}
	srv := &Server{Store: st}
	srv.NoteTeams(ctx)
	events, err := st.Events(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || !strings.Contains(events[0].Message, "Wolves is on at") {
		t.Fatalf("events %+v", events)
	}
	srv.NoteTeams(ctx)
	events, err = st.Events(ctx, 10)
	if err != nil || len(events) != 1 {
		t.Fatalf("announced again: %+v %v", events, err)
	}
}
