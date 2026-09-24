package store

import (
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestSearchFindsListingsAndRecordings(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	if err := s.UpsertDevice(ctx, hdhr.Device{DeviceID: "D", FriendlyName: "DUO", BaseURL: "http://127.0.0.1", TunerCount: 1}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF"},
	}); err != nil {
		t.Fatal(err)
	}
	channels, err := s.Channels(ctx, false)
	if err != nil || len(channels) != 1 {
		t.Fatal(err, channels)
	}
	start := time.Now().Add(time.Hour)
	if err := s.ReplaceAirings(ctx, []Airing{
		{ChannelID: channels[0].ID, Title: "Jeopardy!", Subtitle: "Show 9001", Description: "Answers and questions", Cast: "Ken Jennings", Start: start, End: start.Add(30 * time.Minute)},
		{ChannelID: channels[0].ID, Title: "News", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateRecording(ctx, Recording{ChannelID: channels[0].ID, GuideNumber: "4.1", Title: "Jeopardy!", Status: "recorded", StartedAt: time.Now().Add(-time.Hour), Path: "x.ts"}); err != nil {
		t.Fatal(err)
	}
	airings, recordings, err := s.Search(ctx, "jeop", time.Now(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(airings) != 1 || airings[0].Title != "Jeopardy!" || airings[0].GuideNumber != "4.1" || len(recordings) != 1 {
		t.Fatalf("airings %+v recordings %+v", airings, recordings)
	}
	byCast, _, err := s.Search(ctx, "Ken", time.Now(), 20)
	if err != nil || len(byCast) != 1 {
		t.Fatal(err, byCast)
	}
	none, norec, err := s.Search(ctx, `" OR 1=1`, time.Now(), 20)
	if err != nil || len(none) != 0 || len(norec) != 0 {
		t.Fatal(err, none, norec)
	}
}
