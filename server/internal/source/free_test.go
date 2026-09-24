package source

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPrepareFreeDropsDRM(t *testing.T) {
	raw := `#EXTM3U
#EXTINF:-1 tvg-id="news" tvg-logo="http://example/a.png" group-title="News",News
http://example/news.ts
#EXTINF:-1 drm="1" tvg-id="locked",Locked
http://example/locked.ts
#EXTINF:-1 group-title="DRM",Also locked
drm:widevine
#KODIPROP:inputstream.adaptive.license_type=com.widevine.alpha
#EXTINF:-1 tvg-id="ok",Clear
http://example/clear.ts
`
	kept, skipped := PrepareFree(ParseM3U(strings.NewReader(raw)))
	if skipped != 2 || len(kept) != 2 {
		t.Fatalf("kept %d skipped %d", len(kept), skipped)
	}
	if kept[0].Name != "News" || kept[0].Logo == "" || kept[0].Group != FreeGroup {
		t.Fatalf("news %+v", kept[0])
	}
	if kept[1].DRM || kept[1].Group != FreeGroup {
		t.Fatalf("clear %+v", kept[1])
	}
}

func TestFindFreeFastChannels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/feeds/default/m3u":
			_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-logo=\"http://example/a.png\",News\nhttp://example/a.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	if i := strings.LastIndex(host, ":"); i >= 0 {
		host = host[:i]
	}
	// The fake is not on 5523. FeedByKind still builds the real path.
	feed, err := FeedByKind("fastchannels", srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(feed.Playlist, "/feeds/default/m3u") || !strings.HasSuffix(feed.Guide, "/feeds/default/epg.xml") {
		t.Fatalf("%+v", feed)
	}
	body, err := ReadPlaylist(t.Context(), srv.URL+"/feeds/default/m3u")
	if err != nil {
		t.Fatal(err)
	}
	kept, skipped := PrepareFree(ParseM3U(strings.NewReader(string(body))))
	if skipped != 0 || len(kept) != 1 || kept[0].Logo == "" || kept[0].Group != FreeGroup {
		t.Fatalf("kept %+v skipped %d", kept, skipped)
	}
	_ = host
}
