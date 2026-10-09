package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestSearchReturnsEpisodeAndPlayhead(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := t.Context()
	id, err := s.CreateRecording(ctx, Recording{
		Title: "Sitcom", Status: "complete", Path: "a.ts",
		StartedAt: time.Now().Add(-time.Hour), Season: 2, Episode: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetDuration(ctx, id, 100); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveProgress(ctx, id, 99); err != nil {
		t.Fatal(err)
	}
	_, recs, err := s.Search(ctx, "sitcom", time.Now(), 20)
	if err != nil || len(recs) != 1 {
		t.Fatal(err, recs)
	}
	if recs[0].Season != 2 || recs[0].Episode != 5 || recs[0].Position != 99 || !recs[0].Played() {
		t.Fatalf("%+v", recs[0])
	}
}

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
	// Two words: one in the title, one in the subtitle.
	both, _, err := s.Search(ctx, "jeop show", time.Now(), 20)
	if err != nil || len(both) != 1 {
		t.Fatal(err, both)
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

func TestSearchFindsAnEncryptedStationsShowOnce(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	flex := hdhr.Device{DeviceID: "FLEX1", BaseURL: "http://flex", LineupURL: "http://flex/lineup.json", TunerCount: 4}
	if err := s.UpsertDevice(ctx, flex, []hdhr.Channel{
		{GuideNumber: "5.1", GuideName: "KQRSDT1", VideoCodec: "MPEG2", AudioCodec: "AC3", StreamURL: "http://flex:5004/auto/v5.1"},
		{GuideNumber: "105.1", GuideName: "KQRS", Protected: true, VideoCodec: "HEVC", AudioCodec: "AC4", StreamURL: "http://flex:5004/auto/v105.1"},
	}); err != nil {
		t.Fatal(err)
	}
	got := byNumber(t, s, ctx)
	clear, sealed := got["5.1"][0], got["105.1"][0]
	if sealed.PlaysAs != clear.ID {
		t.Fatalf("105.1 plays as %d, want %d", sealed.PlaysAs, clear.ID)
	}
	start := time.Now().Add(time.Hour)
	if err := s.ReplaceAirings(ctx, []Airing{
		{ChannelID: clear.ID, Title: "Evening News", Start: start, End: start.Add(30 * time.Minute)},
		{ChannelID: sealed.ID, Title: "Evening News", Start: start, End: start.Add(30 * time.Minute)},
		{ChannelID: sealed.ID, Title: "Late News", Start: start.Add(time.Hour), End: start.Add(90 * time.Minute)},
	}); err != nil {
		t.Fatal(err)
	}
	hits, _, err := s.Search(ctx, "news", time.Now(), 20)
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	for _, hit := range hits {
		seen = append(seen, hit.GuideNumber+" "+hit.Title)
	}
	if len(hits) != 2 || seen[0] != "5.1 Evening News" || seen[1] != "105.1 Late News" {
		t.Fatalf("hits %v, want the show both list once on 5.1 and the one only 105.1 lists", seen)
	}
}
