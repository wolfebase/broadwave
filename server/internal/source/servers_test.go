package source

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestTVHeadendImport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ada" || pass != "s3cret" {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		if r.URL.Path != "/playlist/channels" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1 tvg-id=\"news\",Local News\nhttp://example/news.ts\n"))
	}))
	defer srv.Close()
	body, loc, guide, err := TVHeadend(t.Context(), srv.URL, "ada", "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "Local News") {
		t.Fatal(string(body))
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	if loc != "http://ada:s3cret@"+host+"/playlist/channels" || guide != "http://ada:s3cret@"+host+"/xmltv/channels" {
		t.Fatalf("loc %s guide %s", loc, guide)
	}
	if _, _, _, err := TVHeadend(t.Context(), srv.URL, "ada", "wrong"); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("rejected login: %v", err)
	}
}

func TestTVHeadendRejectsNonPlaylist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || user != "ada" || pass != "s3cret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/playlist/channels" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>not a playlist</html>"))
	}))
	defer srv.Close()
	if _, _, _, err := TVHeadend(t.Context(), srv.URL, "ada", "wrong"); err == nil || !strings.Contains(err.Error(), "login") {
		t.Fatalf("rejected login: %v", err)
	}
	if _, _, _, err := TVHeadend(t.Context(), srv.URL, "ada", "s3cret"); err == nil || !strings.Contains(err.Error(), "channel list") {
		t.Fatalf("not an m3u: %v", err)
	}
}

func TestTVHeadendContainer(t *testing.T) {
	base := os.Getenv("WG_TVH")
	if base == "" {
		t.Skip("set WG_TVH to a local tvheadend")
	}
	body, _, guide, err := TVHeadend(t.Context(), base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytesHasM3U(body) {
		t.Fatalf("container playlist: %s", clip(body))
	}
	guideOK(t, guide, "<tv")
}

func TestEmulatorContainer(t *testing.T) {
	cases := []struct {
		env, kind string
		wantInf   bool
	}{
		{"WG_THREADFIN", "threadfin", false},
		{"WG_ERSATZTV", "ersatztv", true},
		{"WG_DISPATCHARR", "dispatcharr", false},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			base := os.Getenv(tc.env)
			if base == "" {
				t.Skip("set " + tc.env + " to a local " + tc.kind)
			}
			body, loc, guide, err := EmulatorM3U(t.Context(), tc.kind, base)
			if err != nil {
				t.Fatal(err)
			}
			if !bytesHasM3U(body) || (tc.wantInf && !strings.Contains(string(body), "#EXTINF")) {
				t.Fatalf("container playlist: %s", clip(body))
			}
			if !strings.Contains(loc, "://") || !strings.Contains(guide, "://") {
				t.Fatalf("loc %s guide %s", loc, guide)
			}
			if tc.kind != "threadfin" {
				guideOK(t, guide, "<tv")
			}
		})
	}
}

func TestChannelsDVRContainer(t *testing.T) {
	base := os.Getenv("WG_CHANNELS")
	if base == "" {
		t.Skip("set WG_CHANNELS to a local Channels DVR")
	}
	body, loc, guide, err := ChannelsDVR(t.Context(), base)
	if err != nil {
		t.Fatal(err)
	}
	if !bytesHasM3U(body) || !strings.Contains(string(body), "#EXTINF") {
		t.Fatalf("container playlist: %s", clip(body))
	}
	if !strings.Contains(loc, "format=ts") || !strings.Contains(loc, "codec=copy") || !strings.Contains(guide, "/guide/xmltv") {
		t.Fatalf("loc %s guide %s", loc, guide)
	}
	guideOK(t, guide, "<channel")
}

func guideOK(t *testing.T, guide, want string) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, guide, nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		t.Fatal(err)
	}
	if res.StatusCode != http.StatusOK || !strings.Contains(string(body), want) {
		t.Fatalf("guide status %d: %s", res.StatusCode, clip(body))
	}
}

func clip(body []byte) string {
	if len(body) > 180 {
		body = body[:180]
	}
	return string(body)
}

func TestChannelsDVRImport(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RequestURI()
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,ABC\nhttp://example/abc.ts\n"))
	}))
	defer srv.Close()
	body, loc, guide, err := ChannelsDVR(t.Context(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "ABC") || !strings.Contains(got, "format=ts") || !strings.Contains(got, "codec=copy") {
		t.Fatalf("uri %s body %s", got, body)
	}
	if !strings.Contains(loc, "/devices/ANY/channels.m3u") || !strings.Contains(guide, "/devices/ANY/guide/xmltv") {
		t.Fatalf("loc %s guide %s", loc, guide)
	}
}

func TestEmulatorPlaylists(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Path
		_, _ = w.Write([]byte("#EXTM3U\n#EXTINF:-1,News\nhttp://example/news.ts\n"))
	}))
	defer srv.Close()
	for kind, paths := range emulatorPaths {
		body, loc, guide, err := EmulatorM3U(t.Context(), kind, srv.URL)
		if err != nil {
			t.Fatal(kind, err)
		}
		if got != paths[0] || !strings.Contains(string(body), "News") || loc != srv.URL+paths[0] || guide != srv.URL+paths[1] {
			t.Fatalf("%s uri %s loc %s guide %s", kind, got, loc, guide)
		}
	}
}

func TestEmulatorImportCases(t *testing.T) {
	const playlist = "#EXTM3U\n#EXTINF:-1,News\nhttp://example/news.ts\n"
	cases := []struct {
		kind, path, guide string
	}{
		{"threadfin", "/m3u/threadfin.m3u", "/xmltv/threadfin.xml"},
		{"xteve", "/m3u/xteve.m3u", "/xmltv/xteve.xml"},
		{"ersatztv", "/iptv/channels.m3u", "/iptv/xmltv.xml"},
		{"dispatcharr", "/output/m3u", "/output/epg"},
	}
	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			t.Run("playlist", func(t *testing.T) {
				var got string
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					got = r.URL.Path
					if r.URL.Path != tc.path {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(playlist))
				}))
				defer srv.Close()
				body, loc, guide, err := EmulatorM3U(t.Context(), tc.kind, srv.URL)
				if err != nil {
					t.Fatal(err)
				}
				if got != tc.path || string(body) != playlist {
					t.Fatalf("path %s body %q", got, body)
				}
				if loc != srv.URL+tc.path || guide != srv.URL+tc.guide {
					t.Fatalf("loc %s guide %s", loc, guide)
				}
			})
			t.Run("not m3u", func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte("<html>not a playlist</html>"))
				}))
				defer srv.Close()
				if _, _, _, err := EmulatorM3U(t.Context(), tc.kind, srv.URL); err == nil || !strings.Contains(err.Error(), "channel list") {
					t.Fatalf("not an m3u: %v", err)
				}
			})
			t.Run("auth", func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// A playlist body must not hide a refusal.
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = w.Write([]byte(playlist))
				}))
				defer srv.Close()
				_, _, _, err := EmulatorM3U(t.Context(), tc.kind, srv.URL)
				if err == nil || !strings.Contains(err.Error(), "refused") || strings.Contains(err.Error(), "channel list") {
					t.Fatalf("rejected: %v", err)
				}
			})
		})
	}
}
