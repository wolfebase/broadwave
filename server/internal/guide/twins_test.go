package guide

import (
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestShareTwinsCopiesTheOtherBroadcastsListings(t *testing.T) {
	lineup := []store.Channel{
		{ID: 1, DeviceID: "F", GuideNumber: "4.1", GuideName: "KBWV-DT", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{ID: 2, DeviceID: "F", GuideNumber: "104.1", GuideName: "KBWV", VideoCodec: "HEVC", AudioCodec: "AC4"},
		{ID: 3, DeviceID: "F", GuideNumber: "19.1", GuideName: "KRVT-1", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{ID: 4, DeviceID: "F", GuideNumber: "119.1", GuideName: "KRVT", VideoCodec: "HEVC", AudioCodec: "AC4"},
		{ID: 5, DeviceID: "F", GuideNumber: "105.1", GuideName: "WTST", VideoCodec: "HEVC", AudioCodec: "AC4", Protected: true},
		{ID: 6, DeviceID: "F", GuideNumber: "5.1", GuideName: "WTSTDT1", VideoCodec: "MPEG2", AudioCodec: "AC3"},
	}
	at := time.Date(2026, 10, 2, 18, 0, 0, 0, time.UTC)
	rows := []store.Airing{
		{ChannelID: 1, Title: "News", Start: at, End: at.Add(time.Hour)},
		{ChannelID: 1, Title: "Football", Start: at.Add(time.Hour), End: at.Add(4 * time.Hour)},
		{ChannelID: 4, Title: "Nature", Start: at, End: at.Add(time.Hour)},
		{ChannelID: 6, Title: "Game Show", Start: at, End: at.Add(time.Hour)},
	}
	art := map[int64]string{1: "kbwv.png"}
	got := map[int64][]string{}
	for _, r := range ShareTwins(rows, art, lineup) {
		got[r.ChannelID] = append(got[r.ChannelID], r.Title)
	}
	if g := got[2]; len(g) != 2 || g[0] != "News" || g[1] != "Football" {
		t.Fatalf("104.1 got %v, want 4.1's listings", g)
	}
	if g := got[3]; len(g) != 1 || g[0] != "Nature" {
		t.Fatalf("19.1 got %v, want 119.1's listings when only the 3.0 channel was listed", g)
	}
	if len(got[5]) != 0 {
		t.Fatal("an encrypted channel got listings")
	}
	if art[2] != "kbwv.png" {
		t.Fatalf("104.1 logo %q", art[2])
	}
}
