package store

import (
	"path/filepath"
	"testing"
	"time"

	"waveguide/internal/hdhr"
)

func TestSetAiringGamesKeepsTheId(t *testing.T) {
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
	start := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if err := s.ReplaceAirings(ctx, []Airing{
		{ChannelID: channels[0].ID, Title: "Chiefs at Bills", Start: start, End: start.Add(3 * time.Hour)},
		{ChannelID: channels[0].ID, Title: "News", Start: start, End: start.Add(time.Hour)},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.Airings(ctx, start.Add(-time.Minute), start.Add(4*time.Hour))
	if err != nil || len(rows) != 2 {
		t.Fatal(err, rows)
	}
	var game int64
	for _, row := range rows {
		if row.Title == "Chiefs at Bills" {
			game = row.ID
		}
	}
	if err := s.SetAiringGames(ctx, start.Add(-time.Minute), start.Add(4*time.Hour), map[int64]string{game: "401772971"}); err != nil {
		t.Fatal(err)
	}
	rows, err = s.Airings(ctx, start.Add(-time.Minute), start.Add(4*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ID == game && row.GameID != "401772971" {
			t.Fatalf("game id %q", row.GameID)
		}
		if row.ID != game && row.GameID != "" {
			t.Fatalf("news picked up %q", row.GameID)
		}
	}
}
