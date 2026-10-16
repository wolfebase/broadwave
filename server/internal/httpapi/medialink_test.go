package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestMediaLinkPlaysForItsDeviceOnly(t *testing.T) {
	api, h := remoteServer(t)
	now := time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)
	api.Clock = func() time.Time { return now }
	call(t, h, http.MethodPut, "/api/v1/settings", `{"remoteUrl":"tv.example.net"}`, "")
	code := jsonField(t, call(t, h, http.MethodPost, "/api/v1/invites", `{"name":"Mom"}`, "").Body.Bytes(), "code")
	claimed := remoteCall(t, h, http.MethodPost, "/api/v1/pair/claim", `{"code":"`+code+`","name":"Phone","kind":"phone"}`, "")
	token := jsonField(t, claimed.Body.Bytes(), "token")
	devID := jsonField(t, []byte(strings.SplitN(strings.SplitN(claimed.Body.String(), `"device":`, 2)[1], `,"token"`, 2)[0]), "id")

	linked := remoteCall(t, h, http.MethodPost, "/api/v1/media/link", `{"path":"/media/live/28/1080.aac2.broadcast/index.m3u8"}`, token)
	if linked.Code != http.StatusOK {
		t.Fatalf("link %d %s", linked.Code, linked.Body.String())
	}
	link := jsonField(t, linked.Body.Bytes(), "url")
	if !strings.HasPrefix(link, "/media/live/28/1080.aac2.broadcast/index.m3u8?mk=") {
		t.Fatalf("link %q", link)
	}
	u, _ := url.Parse(link)
	mk := u.Query().Get("mk")
	notAuth := func(path string) bool {
		return remoteCall(t, h, http.MethodGet, path, "", "").Code == http.StatusUnauthorized
	}
	// The receiver has no header or cookie. The key opens this channel's
	// playlists and segments, and nothing else.
	if notAuth(link) || notAuth("/media/live/28/720.aac2.broadcast/seg00001.m4s?mk="+url.QueryEscape(mk)) {
		t.Fatal("the link did not open its own channel")
	}
	for _, other := range []string{"/media/live/29/1080.aac2.broadcast/index.m3u8", "/media/file/1/index.m3u8", "/api/v1/channels", "/export/lineup.m3u"} {
		if !notAuth(other + "?mk=" + url.QueryEscape(mk)) {
			t.Fatalf("the link opened %s", other)
		}
	}
	if !notAuth("/media/live/28/1080.aac2.broadcast/index.m3u8?mk=" + url.QueryEscape(mk[:len(mk)-2]+"AA")) {
		t.Fatal("a changed key opened the channel")
	}
	now = now.Add(25 * time.Hour)
	if !notAuth(link) {
		t.Fatal("an expired link still plays")
	}
	now = now.Add(-25 * time.Hour)
	if got := call(t, h, http.MethodDelete, "/api/v1/clients/"+devID, "", ""); got.Code != http.StatusOK {
		t.Fatalf("revoke %d", got.Code)
	}
	if !notAuth(link) {
		t.Fatal("a revoked device's link still plays")
	}
	// At home without sign-in there is no device, and the path comes back as is.
	if got := jsonField(t, call(t, h, http.MethodPost, "/api/v1/media/link", `{"path":"/media/live/28/x/index.m3u8"}`, "").Body.Bytes(), "url"); got != "/media/live/28/x/index.m3u8" {
		t.Fatalf("home link %q", got)
	}
	if got := call(t, h, http.MethodPost, "/api/v1/media/link", `{"path":"https://evil.example/x"}`, ""); got.Code != http.StatusBadRequest {
		t.Fatalf("outside path %d", got.Code)
	}
}

func TestKeyedPlaylistNamesEveryURL(t *testing.T) {
	in := `#EXTM3U
#EXT-X-VERSION:9
#EXT-X-MAP:URI="init.mp4"
#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="Main",URI="audio-2.m3u8"
#EXT-X-PART:DURATION=0.5,URI="seg00001.0.m4s",INDEPENDENT=YES
#EXTINF:2.000,
seg00001.m4s
#EXT-X-PRELOAD-HINT:TYPE=PART,URI="seg00002.0.m4s"
#EXT-X-RENDITION-REPORT:URI="../720/index.m3u8?_HLS_msn=3",LAST-MSN=3
index.m3u8?_HLS_msn=4
https://cdn.example/elsewhere.m4s
`
	out := string(keyPlaylist([]byte(in), "a.b+c"))
	for _, want := range []string{`URI="init.mp4?mk=a.b%2Bc"`, `URI="audio-2.m3u8?mk=a.b%2Bc"`, `URI="seg00001.0.m4s?mk=a.b%2Bc"`,
		"\nseg00001.m4s?mk=a.b%2Bc\n", `URI="seg00002.0.m4s?mk=a.b%2Bc"`, `URI="../720/index.m3u8?_HLS_msn=3&mk=a.b%2Bc"`,
		"\nindex.m3u8?_HLS_msn=4&mk=a.b%2Bc\n", "\nhttps://cdn.example/elsewhere.m4s\n", "#EXTINF:2.000,\n"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in\n%s", want, out)
		}
	}
}

func TestKeyedPlaylistPassesSegmentsThrough(t *testing.T) {
	rec := httptest.NewRecorder()
	kp := &keyedPlaylist{ResponseWriter: rec, mk: "k"}
	kp.Header().Set("Content-Type", "video/mp4")
	kp.Header().Set("Content-Length", "3")
	_, _ = kp.Write([]byte("abc"))
	kp.finish()
	if rec.Body.String() != "abc" || rec.Header().Get("Content-Length") != "3" {
		t.Fatalf("segment %q %v", rec.Body.String(), rec.Header())
	}
	rec = httptest.NewRecorder()
	kp = &keyedPlaylist{ResponseWriter: rec, mk: "k"}
	kp.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	kp.Header().Set("Content-Length", "13")
	kp.WriteHeader(http.StatusOK)
	_, _ = kp.Write([]byte("#EXTM3U\nseg.m4s\n"))
	kp.finish()
	if rec.Body.String() != "#EXTM3U\nseg.m4s?mk=k\n" || rec.Header().Get("Content-Length") != "" {
		t.Fatalf("playlist %q %v", rec.Body.String(), rec.Header())
	}
}

func TestOutsideViewersGetAdaptive(t *testing.T) {
	api, _ := remoteServer(t)
	if api.adaptive(context.Background()) {
		t.Fatal("adaptive on at home by default")
	}
	if !api.adaptive(withOutside(context.Background())) {
		t.Fatal("adaptive off outside")
	}
}
