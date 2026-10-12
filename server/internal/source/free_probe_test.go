package source

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/fetchguard"
)

func TestProbeFreeReadsAM3U(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlist.m3u" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,News 4\nhttp://example/news.ts\n"))
	}))
	defer srv.Close()
	host, portNum := loopbackPort(t, srv.URL)
	fp := freePort{port: portNum, kind: "pluto", name: "Pluto", playlist: "/playlist.m3u", guide: "/epg.xml"}
	feed, ok := probeFree(t.Context(), fetchguard.Client(time.Second), host, fp)
	if !ok {
		t.Fatal("not found")
	}
	if feed.Kind != "pluto" || feed.Name != "Pluto" || feed.Addr != srv.URL || feed.Playlist != srv.URL+"/playlist.m3u" || feed.Guide != srv.URL+"/epg.xml" {
		t.Fatalf("%+v", feed)
	}
	closed := freePort{port: 9, kind: "pluto", name: "Pluto", playlist: "/playlist.m3u", guide: "/epg.xml"}
	if _, ok := probeFree(t.Context(), fetchguard.Client(time.Second), "127.0.0.1", closed); ok {
		t.Fatal("a closed port answered")
	}
}

func TestProbeFreeFallsThroughToTheFastChannelsAPI(t *testing.T) {
	playlistStatus := http.StatusOK
	playlistBody := "not a playlist"
	apiStatus := http.StatusOK
	apiBody := `["/feeds/default"]`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feeds/default/m3u":
			w.WriteHeader(playlistStatus)
			_, _ = w.Write([]byte(playlistBody))
		case "/api/feeds":
			w.WriteHeader(apiStatus)
			_, _ = w.Write([]byte(apiBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host, portNum := loopbackPort(t, srv.URL)
	client := fetchguard.Client(time.Second)
	fp := freePort{
		port: portNum, kind: "fastchannels", name: "FastChannels",
		playlist: "/feeds/default/m3u", guide: "/feeds/default/epg.xml",
	}
	feed, ok := probeFree(t.Context(), client, host, fp)
	if !ok {
		t.Fatal("api")
	}
	if feed.Kind != "fastchannels" || feed.Name != "FastChannels" || feed.Addr != srv.URL || feed.Playlist != srv.URL+fp.playlist || feed.Guide != srv.URL+fp.guide {
		t.Fatalf("%+v", feed)
	}

	apiStatus = http.StatusNotFound
	if _, ok := probeFree(t.Context(), client, host, fp); ok {
		t.Fatal("404 api")
	}
	apiStatus = http.StatusOK
	apiBody = `{"ok":true}`
	if _, ok := probeFree(t.Context(), client, host, fp); ok {
		t.Fatal("api body")
	}

	apiBody = `["/feeds/default"]`
	fp.kind = "pluto"
	fp.name = "Pluto"
	if _, ok := probeFree(t.Context(), client, host, fp); ok {
		t.Fatal("other kind")
	}

	// An error page that starts with the playlist marker is still not a feed.
	fp.kind = "samsung"
	fp.name = "Samsung TV Plus"
	playlistStatus = http.StatusNotFound
	playlistBody = "#EXTM3U\n"
	if _, ok := probeFree(t.Context(), client, host, fp); ok {
		t.Fatal("error status")
	}
	if fastChannelsAPI(t.Context(), client, "http://127.0.0.1:9") {
		t.Fatal("closed api")
	}
}

func TestFindFreeIgnoresBlankHostsAndACancelledLook(t *testing.T) {
	if FindFree(t.Context(), nil) != nil {
		t.Fatal("nil hosts")
	}
	if FindFree(t.Context(), []string{}) != nil {
		t.Fatal("empty hosts")
	}
	if FindFree(t.Context(), []string{"", " ", "\t"}) != nil {
		t.Fatal("blank hosts")
	}
	done, stop := context.WithCancel(t.Context())
	stop()
	started := time.Now()
	if got := FindFree(done, []string{"127.0.0.1", " 127.0.0.1"}); got != nil {
		t.Fatalf("cancelled %+v", got)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("cancelled took %s", time.Since(started))
	}
}

func TestFeedByKindNamesEachGenerator(t *testing.T) {
	feed, err := FeedByKind("fastchannels", "http://127.0.0.1:5523/")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Kind != "fastchannels" || feed.Name != "FastChannels" || feed.Addr != "http://127.0.0.1:5523" || feed.Playlist != "http://127.0.0.1:5523/feeds/default/m3u" || feed.Guide != "http://127.0.0.1:5523/feeds/default/epg.xml" {
		t.Fatalf("%+v", feed)
	}
	feed, err = FeedByKind("pluto", "127.0.0.1:7777")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Kind != "pluto" || feed.Name != "Pluto" || feed.Addr != "http://127.0.0.1:7777" || feed.Playlist != "http://127.0.0.1:7777/playlist.m3u" || feed.Guide != "http://127.0.0.1:7777/epg.xml" {
		t.Fatalf("%+v", feed)
	}
	feed, err = FeedByKind(" Samsung ", "127.0.0.1:8182/")
	if err != nil {
		t.Fatal(err)
	}
	if feed.Kind != "samsung" || feed.Name != "Samsung TV Plus" || feed.Addr != "http://127.0.0.1:8182" || feed.Playlist != "http://127.0.0.1:8182/playlist.m3u8?regions=us" || feed.Guide != "http://127.0.0.1:8182/epg.xml" {
		t.Fatalf("%+v", feed)
	}
	if _, err := FeedByKind("radio", "127.0.0.1:9"); err == nil || err.Error() != "that is not a free-channel server" {
		t.Fatal(err)
	}
}

func TestPlaylistPickListsGroupsOrChannels(t *testing.T) {
	grouped := []Entry{
		{Name: "News 4", Group: "News", ID: "n4", Number: "4"},
		{Name: "City Sports", Group: "Sports", ID: "cs", Number: "7"},
		{Name: "Also News", Group: "News", ID: "n5", Number: "5"},
	}
	got := PlaylistPick(grouped, "Keep groups: News, Sports.")
	if got["pick"] != true || got["count"] != 3 || got["message"] != "Keep groups: News, Sports." {
		t.Fatalf("%+v", got)
	}
	groups, ok := got["groups"].([]string)
	if !ok || len(groups) != 2 || groups[0] != "News" || groups[1] != "Sports" {
		t.Fatalf("%+v", got["groups"])
	}
	if _, ok := got["channels"]; ok {
		t.Fatal("groups should not include channels")
	}
	flat := []Entry{
		{Name: "News 4", ID: "n4", Number: "4"},
		{Name: "City Sports", ID: "cs", Number: "7"},
	}
	got = PlaylistPick(flat, "This playlist has 2 channels.")
	if got["pick"] != true || got["count"] != 2 || got["message"] != "This playlist has 2 channels." {
		t.Fatalf("%+v", got)
	}
	groups, ok = got["groups"].([]string)
	if !ok || len(groups) != 0 {
		t.Fatalf("%+v", got["groups"])
	}
	channels, ok := got["channels"].([]map[string]string)
	if !ok || len(channels) != 2 {
		t.Fatalf("%+v", got["channels"])
	}
	if channels[0]["name"] != "News 4" || channels[0]["id"] != "n4" || channels[0]["number"] != "4" {
		t.Fatalf("%+v", channels[0])
	}
	if channels[1]["name"] != "City Sports" || channels[1]["id"] != "cs" || channels[1]["number"] != "7" {
		t.Fatalf("%+v", channels[1])
	}
}

func TestParseM3UAppliesStreamOptions(t *testing.T) {
	raw := `#EXTM3U
#EXTINF:-1,News 4
#EXTVLCOPT:http-user-agent=Broadwave
#KODIPROP:http-referrer=http://example/news
#EXTVLCOPT:unknown-key=1
#KODIPROP:no-equals
http://example/news.ts
#EXTINF:-1,City Sports
#KODIPROP:http-referer=http://example/sports
#EXTVLCOPT:inputstream.adaptive.license_type=com.widevine.alpha
http://example/sports.ts
#EXTINF:-1,Night
#KODIPROP:inputstream.adaptive.license_type=com.microsoft.playready
http://example/night.ts
#EXTINF:-1,Clear
#KODIPROP:inputstream.adaptive.license_type=other
http://example/clear.ts
`
	got := ParseM3U(strings.NewReader(raw))
	if len(got) != 4 {
		t.Fatalf("%d %+v", len(got), got)
	}
	if got[0].Name != "News 4" || got[0].UserAgent != "Broadwave" || got[0].Referrer != "http://example/news" || got[0].DRM {
		t.Fatalf("%+v", got[0])
	}
	if got[1].Name != "City Sports" || got[1].Referrer != "http://example/sports" || !got[1].DRM {
		t.Fatalf("%+v", got[1])
	}
	if got[2].Name != "Night" || !got[2].DRM || got[2].UserAgent != "" || got[2].Referrer != "" {
		t.Fatalf("%+v", got[2])
	}
	if got[3].Name != "Clear" || got[3].DRM {
		t.Fatalf("%+v", got[3])
	}
}

func TestXtreamFromStoredKeepsTheLoginOffThePath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/player_api.php" || r.URL.User != nil {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("username") != "viewer" || r.URL.Query().Get("password") != "secret-pass" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		switch r.URL.Query().Get("action") {
		case "get_live_categories":
			_, _ = w.Write([]byte(`[{"category_id":"2","category_name":"Sports"}]`))
		case "get_live_streams":
			_, _ = w.Write([]byte(`[{"num":7,"name":"City Sports","stream_id":42,"stream_icon":"http://example/s.png","epg_channel_id":"sports","category_id":"2"}]`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if _, _, err := XtreamFromStored(t.Context(), srv.URL); err == nil || err.Error() != "This Xtream source has no saved login. Add it again." {
		t.Fatal(err)
	}
	stored, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	stored.User = url.UserPassword("viewer", "secret-pass")
	stored.Path = "/live/viewer/secret-pass/42.ts"
	stored.RawQuery = "leftover=1"
	body, guide, err := XtreamFromStored(t.Context(), stored.String())
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if !strings.Contains(text, "City Sports") || !strings.Contains(text, "/live/viewer/secret-pass/42.ts") || !strings.Contains(text, `group-title="Sports"`) {
		t.Fatal(text)
	}
	if strings.Contains(guide, "viewer:secret-pass@") || !strings.Contains(guide, "password=secret-pass") {
		t.Fatal(guide)
	}
	got := ParseM3U(strings.NewReader(text))
	if len(got) != 1 || got[0].Name != "City Sports" || got[0].Number != "7" || got[0].Group != "Sports" || !strings.HasSuffix(got[0].URL, "/live/viewer/secret-pass/42.ts") {
		t.Fatalf("%+v", got)
	}
}

func TestAutoStaysOffTheNetwork(t *testing.T) {
	t.Setenv("BROADWAVE_E2E", "1")
	st := openStore(t)
	if _, err := Auto(t.Context(), st, nil, "not-a-tuner.example"); err == nil || err.Error() != "this test server only uses the local fake tuner" {
		t.Fatal(err)
	}
	n, err := Auto(t.Context(), st, nil, "")
	if err != nil || n != 0 {
		t.Fatalf("broadcast %d %v", n, err)
	}
	// Port 9 stays closed. The error has to be the fetch of that one base.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	n, err = Auto(ctx, st, nil, "127.0.0.1:9")
	if err == nil || n != 0 {
		t.Fatalf("%d %v", n, err)
	}
	if strings.Contains(err.Error(), "local fake tuner") || strings.Contains(err.Error(), "No HDHomeRun") || !strings.Contains(err.Error(), "connection refused") {
		t.Fatal(err)
	}
	devices, err := st.Devices(t.Context())
	if err != nil || len(devices) != 0 {
		t.Fatalf("%+v %v", devices, err)
	}
}

func TestCompatibleChecksDiscoverJSON(t *testing.T) {
	okSrv := discoverServer(t, http.StatusOK, `{"DeviceID":"FAKE0001","FriendlyName":"Lab"}`)
	if !compatible(t.Context(), okSrv.URL) {
		t.Fatal("device id")
	}
	plain := discoverServer(t, http.StatusOK, `{"FriendlyName":"Lab"}`)
	if compatible(t.Context(), plain.URL) {
		t.Fatal("no device id")
	}
	missing := discoverServer(t, http.StatusNotFound, `{"DeviceID":"FAKE0001"}`)
	if compatible(t.Context(), missing.URL) {
		t.Fatal("404")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if compatible(ctx, okSrv.URL) {
		t.Fatal("cancelled")
	}
}

func TestBasesForTakesOneHostPort(t *testing.T) {
	t.Setenv("BROADWAVE_E2E", "")
	if _, err := basesFor(t.Context(), "bad host/path"); err == nil || err.Error() != "enter a host or host:port" {
		t.Fatal(err)
	}
	t.Setenv("BROADWAVE_E2E", "1")
	bases, err := basesFor(t.Context(), "127.0.0.1:19201")
	if err != nil {
		t.Fatal(err)
	}
	if len(bases) != 1 || bases[0] != "http://127.0.0.1:19201" {
		t.Fatalf("%v", bases)
	}
	// A slash is not a loopback host, so the harness answers before the host check.
	if _, err := basesFor(t.Context(), "bad host/path"); err == nil || err.Error() != "this test server only uses the local fake tuner" {
		t.Fatal(err)
	}
}

func loopbackPort(t *testing.T, raw string) (string, int) {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if u.Hostname() != "127.0.0.1" {
		t.Fatalf("host %s", u.Hostname())
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return u.Hostname(), port
}

func discoverServer(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/discover.json" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}
