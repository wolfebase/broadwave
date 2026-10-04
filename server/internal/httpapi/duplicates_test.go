package httpapi

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestTwoTunersOnOneAntennaShowEachChannelOnce(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	for _, id := range []string{"AAA1", "BBB2"} {
		if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: id, BaseURL: "http://" + id, TunerCount: 2}, []hdhr.Channel{
			{GuideNumber: "4.1", GuideName: "KBWV-DT", StreamURL: "http://" + id + ":5004/auto/v4.1"},
			{GuideNumber: "5.1", GuideName: "WTSTDT1", StreamURL: "http://" + id + ":5004/auto/v5.1"},
		}); err != nil {
			t.Fatal(err)
		}
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}}).Handler()
	list := func(path string) []store.Channel {
		var body struct {
			Channels []store.Channel `json:"channels"`
		}
		if err := json.Unmarshal(get(t, h, path).Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Channels
	}

	guide := list("/api/v1/channels?guide=1")
	if len(guide) != 2 || guide[0].GuideNumber != "4.1" || guide[1].GuideNumber != "5.1" {
		t.Fatalf("guide = %+v, want 4.1 and 5.1 once", guide)
	}
	all := list("/api/v1/channels")
	if len(all) != 4 {
		t.Fatalf("all channels = %d, want 4", len(all))
	}
	shown := map[int64]bool{guide[0].ID: true, guide[1].ID: true}
	for _, ch := range all {
		if !shown[ch.ID] && !shown[ch.SameAs] {
			t.Fatalf("%s on %s points at %d, not a shown row", ch.GuideNumber, ch.DeviceID, ch.SameAs)
		}
	}
	// The M3U for Plex, Jellyfin, and Channels lists each once too.
	if n := strings.Count(get(t, h, "/export/lineup.m3u").Body.String(), "#EXTINF"); n != 2 {
		t.Fatalf("lineup.m3u = %d entries, want 2", n)
	}
}
