package store

import (
	"context"
	"strings"
	"testing"

	"broadwave/internal/hdhr"
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

func TestOtherDeviceIsTheFailover(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	a := hdhr.Device{DeviceID: "AAAA", FriendlyName: "First", BaseURL: "http://192.168.1.20", TunerCount: 2}
	b := hdhr.Device{DeviceID: "BBBB", FriendlyName: "Second", BaseURL: "http://192.168.1.30", TunerCount: 2}
	if err := st.UpsertDevice(ctx, a, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://a/4.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(ctx, b, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://b/4.1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE devices SET priority=1 WHERE device_id='AAAA'`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE devices SET priority=2 WHERE device_id='BBBB'`); err != nil {
		t.Fatal(err)
	}
	got, err := st.OtherDevices(ctx, "4.1", "AAAA")
	if err != nil || len(got) != 1 || got[0] != "http://192.168.1.30" {
		t.Fatalf("%v %v", got, err)
	}
	stream, err := st.ChannelStreamURL(ctx, "http://192.168.1.30", "4.1")
	if err != nil || stream != "http://b/4.1" {
		t.Fatalf("stream %q %v", stream, err)
	}
	locked := []hdhr.Channel{{GuideNumber: "702", GuideName: "HBO", StreamURL: "http://a/702", Protected: true}}
	if err := st.UpsertDevice(ctx, a, append([]hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://a/4.1"}}, locked...)); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range channels {
		if item.GuideNumber == "702" {
			t.Fatalf("copy protected channel was offered: %+v", item)
		}
	}
}

func TestGuideKeyKeepsTheChannelWhenTheAddressChanges(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "src-9", FriendlyName: "Playlist", BaseURL: "source"}
	first := []hdhr.Channel{{GuideNumber: "801", GuideName: "News", StreamURL: "http://old/news.ts", GuideKey: "wdaf"}}
	if err := st.UpsertDevice(context.Background(), dev, first); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	id := channels[0].ID
	moved := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://new/abc.ts", GuideKey: "wdaf"}}
	if err := st.UpsertDevice(context.Background(), dev, moved); err != nil {
		t.Fatal(err)
	}
	channels, err = st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 || channels[0].ID != id || channels[0].GuideNumber != "4.1" {
		t.Fatalf("%+v %v", channels, err)
	}
}

func TestChannelKeepsTheStreamHeaders(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "src-9", FriendlyName: "Playlist", BaseURL: "source"}
	row := hdhr.Channel{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts", UserAgent: "Broadwave", Referrer: "http://example/"}
	if err := st.UpsertDevice(context.Background(), dev, []hdhr.Channel{row}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	got, err := st.SourceChannel(context.Background(), channels[0].ID)
	if err != nil || got.UserAgent != "Broadwave" || got.Referrer != "http://example/" {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestAlternateChannelFollowsPriority(t *testing.T) {
	st := openTestStore(t)
	low := hdhr.Device{DeviceID: "src-low", FriendlyName: "Second", BaseURL: "http://second"}
	high := hdhr.Device{DeviceID: "src-high", FriendlyName: "First", BaseURL: "http://first"}
	if err := st.UpsertDevice(context.Background(), high, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://first/4.1"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertDevice(context.Background(), low, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://second/4.1"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE devices SET priority=1 WHERE device_id='src-low'`); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 2 {
		t.Fatalf("%+v %v", channels, err)
	}
	var first int64
	for _, ch := range channels {
		if ch.DeviceID == "src-high" {
			first = ch.ID
		}
	}
	alts, err := st.AlternateChannels(context.Background(), "4.1", first)
	if err != nil || len(alts) != 1 {
		t.Fatalf("%v %v", alts, err)
	}
	got, err := st.SourceChannel(context.Background(), alts[0])
	if err != nil || got.DeviceID != "src-low" {
		t.Fatalf("%+v %v", got, err)
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

func TestMaskedSourceFetchesWithItsLogin(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	playlist := "http://user:s3cret@example/pl.m3u?token=abc"
	guide := "http://example/xmltv.php?password=s3cret&username=user"
	item, err := st.AddSource(ctx, "xtream", "IPTV", playlist, guide)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(item.XMLTV, "s3cret") {
		t.Fatalf("guide leaked the password: %s", item.XMLTV)
	}
	if got := st.FetchURL(ctx, item.ID, item.URL); got != playlist {
		t.Fatalf("playlist fetch = %q", got)
	}
	if got := st.FetchURL(ctx, item.ID, item.XMLTV); got != guide {
		t.Fatalf("guide fetch = %q", got)
	}
	if got := st.FetchURL(ctx, item.ID, "http://example/plain.m3u"); got != "http://example/plain.m3u" {
		t.Fatalf("plain fetch = %q", got)
	}
}

func TestMaskURLMatchesTheStoredForm(t *testing.T) {
	raw := "http://ops3user:ops3-fixture-password@playlist.example/pl.m3u?password=ops3-fixture-password"
	public, secret := maskURL(raw)
	if MaskURL(raw) != public || secret != raw {
		t.Fatalf("wrapper %q public %q", MaskURL(raw), public)
	}
	if strings.Contains(public, "ops3-fixture-password") {
		t.Fatalf("leaked %s", public)
	}
	if MaskURL(public) != public {
		t.Fatalf("unstable %q vs %q", MaskURL(public), public)
	}
}

func TestOldXtreamGuideUsesThePlaylistLogin(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	item, err := st.AddSource(ctx, "xtream", "IPTV", "http://user:s3cret@example", "http://example/xmltv.php?password=s3cret&username=user")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE source_secrets SET guide_secret='' WHERE source_id=?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if got := st.FetchURL(ctx, item.ID, item.XMLTV); got != "http://example/xmltv.php?password=s3cret&username=user" {
		t.Fatalf("guide fetch = %q", got)
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
