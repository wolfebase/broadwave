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
		{GuideNumber: "4.1", GuideName: "KBWV"},
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

func TestSearchListsAChannelOnceAndSkipsHiddenOnes(t *testing.T) {
	s, ctx := openTwoTuners(t)
	all, err := s.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().Add(time.Hour)
	var airings []Airing
	for _, ch := range all {
		airings = append(airings, Airing{ChannelID: ch.ID, Title: "News at " + ch.GuideNumber, Start: start, End: start.Add(30 * time.Minute)})
	}
	if err := s.ReplaceAirings(ctx, airings); err != nil {
		t.Fatal(err)
	}
	hide := true
	if _, err := s.PatchChannel(ctx, rowFor(t, all, "CCC3", "5.2").ID, ChannelPatch{Hidden: &hide}); err != nil {
		t.Fatal(err)
	}
	got, _, err := s.Search(ctx, "news", time.Now(), 20)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]int{}
	for _, hit := range got {
		seen[hit.GuideNumber+" "+hit.ChannelName]++
	}
	if len(got) != 2 || seen["5.1 WTSTDT1"] != 1 || seen["5.2 Rivers"] != 1 {
		t.Fatalf("hits %v, want 5.1 and 5.2 Rivers once each (two tuners carry them; Mountains is hidden)", seen)
	}
}
