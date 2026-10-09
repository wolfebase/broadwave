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
	if present != 1 || kept.ID != id || kept.GuideName != "News Tonight" || kept.GuideNumber != "900" {
		t.Fatalf("refresh moved the channel: present %d %+v", present, channels)
	}
}

func TestDuplicateStreamURLKeepsBothChannels(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "src-1", FriendlyName: "Playlist", BaseURL: "source"}
	err := st.UpsertDevice(context.Background(), dev, []hdhr.Channel{
		{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/a.ts"},
		{GuideNumber: "802", GuideName: "News HD", StreamURL: "http://example/a.ts"},
	})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ch := range channels {
		if ch.Present {
			got[ch.GuideNumber] = ch.GuideName
		}
	}
	if got["801"] != "News" || got["802"] != "News HD" || len(got) != 2 {
		t.Fatalf("channels %+v", channels)
	}
}

func TestSwappedNumbersFollowTheStream(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "src-1", FriendlyName: "Playlist", BaseURL: "source"}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts"},
		{GuideNumber: "802", GuideName: "Sports", StreamURL: "http://example/sports.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]int64{}
	for _, ch := range before {
		var url string
		if err := st.db.QueryRow(`SELECT stream_url FROM channels WHERE id=?`, ch.ID).Scan(&url); err != nil {
			t.Fatal(err)
		}
		ids[url] = ch.ID
	}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "802", GuideName: "News Moved", StreamURL: "http://example/news.ts"},
		{GuideNumber: "801", GuideName: "Sports Moved", StreamURL: "http://example/sports.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Channel{}
	for _, ch := range after {
		if ch.Present {
			var url string
			if err := st.db.QueryRow(`SELECT stream_url FROM channels WHERE id=?`, ch.ID).Scan(&url); err != nil {
				t.Fatal(err)
			}
			got[url] = ch
		}
	}
	news, sports := got["http://example/news.ts"], got["http://example/sports.ts"]
	if news.ID != ids["http://example/news.ts"] || news.GuideNumber != "802" || news.GuideName != "News Moved" ||
		sports.ID != ids["http://example/sports.ts"] || sports.GuideNumber != "801" || sports.GuideName != "Sports Moved" ||
		len(got) != 2 {
		t.Fatalf("swap lost a channel: %+v", after)
	}
}

func TestAChannelMissingFromOneRefreshKeepsItsNumber(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	dev := hdhr.Device{DeviceID: "src-1", FriendlyName: "Playlist", BaseURL: "source"}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts"},
		{GuideNumber: "802", GuideName: "Sports", StreamURL: "http://example/sports.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	var sportsID int64
	for _, ch := range channels {
		if ch.GuideNumber == "802" {
			sportsID = ch.ID
		}
	}
	if sportsID == 0 {
		t.Fatal("sports channel missing")
	}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	var number string
	var present int
	if err := st.db.QueryRow(`SELECT guide_number, present FROM channels WHERE id=?`, sportsID).Scan(&number, &present); err != nil {
		t.Fatal(err)
	}
	if number != "802" || present != 0 {
		t.Fatalf("absent channel number %q present %d", number, present)
	}
	if err := st.UpsertDevice(ctx, dev, []hdhr.Channel{
		{GuideNumber: "801", GuideName: "News", StreamURL: "http://example/news.ts"},
		{GuideNumber: "802", GuideName: "Sports Tonight", StreamURL: "http://example/sports-new.ts"},
	}); err != nil {
		t.Fatal(err)
	}
	var name, url string
	if err := st.db.QueryRow(`SELECT guide_number, guide_name, stream_url, present FROM channels WHERE id=?`, sportsID).Scan(&number, &name, &url, &present); err != nil {
		t.Fatal(err)
	}
	if number != "802" || name != "Sports Tonight" || url != "http://example/sports-new.ts" || present != 1 {
		t.Fatalf("comeback id %d number %q name %q url %q present %d", sportsID, number, name, url, present)
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
	// A row hidden from the lineup, as a 1.0 channel whose 3.0 twin is shown
	// is, still takes over when the first device is unplugged.
	if _, err := st.db.Exec(`UPDATE channels SET hidden=1 WHERE device_id='BBBB'`); err != nil {
		t.Fatal(err)
	}
	got, err = st.OtherDevices(ctx, "4.1", "AAAA")
	if err != nil || len(got) != 1 {
		t.Fatalf("hidden row was not a failover: %v %v", got, err)
	}
	var firstID int64
	if err := st.db.QueryRow(`SELECT id FROM channels WHERE device_id='AAAA' AND guide_number='4.1'`).Scan(&firstID); err != nil {
		t.Fatal(err)
	}
	if alts, err := st.AlternateChannels(ctx, "4.1", firstID); err != nil || len(alts) != 1 {
		t.Fatalf("hidden row was not an alternate: %v %v", alts, err)
	}
	if _, err := st.db.Exec(`UPDATE channels SET protected=1 WHERE device_id='BBBB'`); err != nil {
		t.Fatal(err)
	}
	if got, err := st.OtherDevices(ctx, "4.1", "AAAA"); err != nil || len(got) != 0 {
		t.Fatalf("protected row was a failover: %v %v", got, err)
	}
	if alts, err := st.AlternateChannels(ctx, "4.1", firstID); err != nil || len(alts) != 0 {
		t.Fatalf("protected row was an alternate: %v %v", alts, err)
	}
	if _, err := st.db.Exec(`UPDATE channels SET hidden=0, protected=0 WHERE device_id='BBBB'`); err != nil {
		t.Fatal(err)
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
	first := []hdhr.Channel{{GuideNumber: "801", GuideName: "News", StreamURL: "http://old/news.ts", GuideKey: "kbwv"}}
	if err := st.UpsertDevice(context.Background(), dev, first); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	id := channels[0].ID
	moved := []hdhr.Channel{{GuideNumber: "4.1", GuideName: "ABC", StreamURL: "http://new/abc.ts", GuideKey: "kbwv"}}
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

func TestChannelKeepsItsMeasuredAudio(t *testing.T) {
	st := openTestStore(t)
	dev := hdhr.Device{DeviceID: "dev-1", FriendlyName: "Tuner", BaseURL: "http://tuner"}
	if err := st.UpsertDevice(context.Background(), dev, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}}); err != nil {
		t.Fatal(err)
	}
	channels, err := st.Channels(context.Background(), false)
	if err != nil || len(channels) != 1 {
		t.Fatalf("%+v %v", channels, err)
	}
	id := channels[0].ID
	const tracks = `[{"pid":52,"role":"main","codec":"ac3","channels":6,"measured":true}]`
	if err := st.SetChannelAudioTracks(context.Background(), id, tracks); err != nil {
		t.Fatal(err)
	}
	// A rescan rewrites the lineup row but keeps what a tune measured.
	if err := st.UpsertDevice(context.Background(), dev, []hdhr.Channel{{GuideNumber: "4.1", GuideName: "KBWV"}}); err != nil {
		t.Fatal(err)
	}
	got, err := st.SourceChannel(context.Background(), id)
	if err != nil || got.AudioTracks != tracks {
		t.Fatalf("%q %v", got.AudioTracks, err)
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

func TestAddSourceRollsBackWhenTheSecretCannotBeStored(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.db.Exec(`CREATE TRIGGER reject_secret BEFORE INSERT ON source_secrets BEGIN SELECT RAISE(ABORT, 'boom'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := st.AddSource(context.Background(), "m3u", "IPTV", "http://user:s3cret@example/pl.m3u", "")
	if err == nil {
		t.Fatal("stored a source whose password was rejected")
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM sources`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("left %d source row(s) after the password could not be stored", n)
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
