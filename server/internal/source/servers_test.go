package source

import (
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
	if !strings.Contains(loc, "ada:") || !strings.Contains(loc, "s3cret") || !strings.Contains(guide, "/xmltv/channels") {
		t.Fatalf("loc %s guide %s", loc, guide)
	}
	if _, _, _, err := TVHeadend(t.Context(), srv.URL, "ada", "wrong"); err == nil {
		t.Fatal("a rejected login should fail")
	}
}

func TestTVHeadendContainer(t *testing.T) {
	base := os.Getenv("WG_TVH")
	if base == "" {
		t.Skip("set WG_TVH to a local tvheadend")
	}
	body, _, _, err := TVHeadend(t.Context(), base, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytesHasM3U(body) {
		t.Fatalf("container playlist: %s", body)
	}
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
