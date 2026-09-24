package store

import (
	"context"
	"strings"
	"testing"

	"waveguide/internal/hdhr"
)

func TestRefreshKeepsTheChannelID(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "src-1", FriendlyName: "Playlist", BaseURL: "source"}
	first := []hdhr.Channel{{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts"}}
	if err := st.UpsertDevice(context.Background(), dev, first); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("channels %+v err %v", channels, err)
	}
	id := channels[0].ID
	renumbered := []hdhr.Channel{{GuideNumber: "900", GuideName: "News Tonight", StreamURL: "http://example/news.ts"}}
	if err := st.UpsertDevice(context.Background(), dev, renumbered); err != nil {
		t.Fatal(err)
	}
	channels, err = st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	var kept Channel
	present := 0
	for _, ch := range channels {
		if ch.Present {
			present++
			kept = ch
		}
	}
	if present != 1 || kept.ID != id || kept.GuideName != "News Tonight" {
		t.Fatalf("refresh moved the channel: present %d %+v", present, channels)
	}
}

func TestNewAddressKeepsTheChannelID(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "10611B4C", FriendlyName: "HDHomeRun", BaseURL: "http://192.168.1.20"}
	first := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://192.168.1.20:5004/auto/v4.1"}}
	if err := st.UpsertDevice(context.Background(), dev, first); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	id := channels[0].ID
	dev.BaseURL = "http://192.168.1.50"
	moved := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://192.168.1.50:5004/auto/v4.1"}}
	if err := st.UpsertDevice(context.Background(), dev, moved); err != nil {
		t.Fatal(err)
	}
	channels, err = st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 || channels[0].ID != id {
		t.Fatalf("%+v %v", channels, err)
	}
	var url string
	if err := st.db.QueryRow(`SELECT stream_url FROM channels WHERE id=?`, id).Scan(&url); err != nil {
		t.Fatal(err)
	}
	if url != moved[0].StreamURL {
		t.Fatalf("address stayed %s", url)
	}
}

func TestSourcePasswordIsMasked(t *testing.T) {
	st := openTestStore(t)
	item, err := st.AddSource(context.Background(), "m3u", "IPTV", "http://user:s3cret@example/pl.m3u?password=s3cret", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(item.URL, "s3cret") {
		t.Fatalf("response leaked the password: %s", item.URL)
	}
	list, err := st.Sources(context.Background())
	if err != nil || len(list) != 1 || strings.Contains(list[0].URL, "s3cret") {
		t.Fatalf("list leaked the password: %+v err %v", list, err)
	}
	var secret string
	if err := st.db.QueryRow(`SELECT secret FROM source_secrets WHERE source_id=?`, item.ID).Scan(&secret); err != nil {
		t.Fatal(err)
	}
	if secret != "http://user:s3cret@example/pl.m3u?password=s3cret" {
		t.Fatalf("secret stored as %q", secret)
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}
