package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/realtime"
)

func init() {
	testHosts = []string{"example.com"}
}

func TestAnotherSiteCannotPostToTheServer(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	post := func(header map[string]string) int {
		req := httptest.NewRequest(http.MethodPost, "http://tv.local:8477/api/v1/backup", strings.NewReader("not a catalog"))
		req.Header.Set("Content-Type", "text/plain")
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// A tab on another site, in a browser that sends Sec-Fetch-Site and in
	// one that sends only Origin.
	if code := post(map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("cross-site restore: %d", code)
	}
	if code := post(map[string]string{"Origin": "https://evil.example"}); code != http.StatusForbidden {
		t.Fatalf("cross-origin restore: %d", code)
	}
	// The web app itself, and the apps and curl, which send no Origin, reach
	// the handler (it refuses the body, not the caller).
	for _, header := range []map[string]string{
		{"Sec-Fetch-Site": "same-origin", "Origin": "http://tv.local:8477"},
		{"Origin": "http://tv.local:8477"},
		{},
	} {
		if code := post(header); code == http.StatusForbidden {
			t.Fatalf("%v refused", header)
		}
	}
	// Reading stays open: another app on the LAN reads the lineup.
	req := httptest.NewRequest(http.MethodGet, "http://tv.local:8477/api/v1/health", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health: %d", rec.Code)
	}
}

func TestARenamedHostIsRefused(t *testing.T) {
	h := (&Server{Store: testStore(t), Hosts: []string{"tv.example.com", ".proxy.example.net", ""}}).Handler()
	status := func(host string, https bool) int {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
		req.Host = host
		if https {
			req.Header.Set("Sec-Fetch-Site", "same-origin")
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	// A page on a public name that resolves to this server could read the
	// backup (it holds source logins) or restore one. Over plain HTTP no
	// browser sends Sec-Fetch-Site, on any port. Some country domains are
	// listed only as wildcards (*.np, *.ck).
	for _, host := range []string{
		"rebind.example.com:8477", "x.duckdns.org:8477", "rebind.example.com:9000", "tv.example.com.rebind.example.org:8477",
		"rebind.evil.com.np:8477", "a.b.ck:8477", "rebind.example.com", "rebind.example.com:80", "rebind.example.com..:8477",
		"rebind.example.com:8477:1", "tus.lan:8477:1", "rebind.example.com%2f:8477",
	} {
		if code := status(host, false); code != http.StatusMisdirectedRequest {
			t.Errorf("%s: %d, want 421", host, code)
		}
	}
	// A browser over HTTPS on a public name is behind a proxy with a
	// certificate for it, but only on the standard port.
	if code := status("rebind.example.com:8477", true); code != http.StatusMisdirectedRequest {
		t.Errorf("https on the server's port: %d", code)
	}
	// Addresses, home names, configured names, and a browser behind an HTTPS proxy.
	for _, host := range []string{
		"192.168.1.2:8477", "[fe80::1]:8477", "[fd00::2]", "[fe80::1%25en0]:8477", "localhost:8477", "tus:8477", "tus.local:8477",
		"TUS.Local.:8477", "tus.lan:8477", "nas.localdomain:8477", "tus.home.arpa:8477", "tus.fritz.box:8477", "tv.example.com:8477",
		"a.proxy.example.net:8477", "tv.example:8477", "",
	} {
		if code := status(host, false); code == http.StatusMisdirectedRequest {
			t.Errorf("%s refused", host)
		}
	}
	for _, host := range []string{"tv.other.example.com", "tv.other.example.com:443"} {
		if code := status(host, true); code == http.StatusMisdirectedRequest {
			t.Errorf("https proxy %s refused", host)
		}
	}
}

func TestAnotherSiteCannotOpenTheSocket(t *testing.T) {
	h := (&Server{Store: testStore(t), Bus: realtime.NewBus()}).Handler()
	open := func(header map[string]string) int {
		req := httptest.NewRequest(http.MethodGet, "http://tv.local:8477/api/v1/ws", nil)
		for k, v := range header {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	for _, header := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site", "Origin": "https://evil.example"},
		{"Sec-Fetch-Site": "same-site", "Origin": "http://other.local:8477"},
		{"Origin": "https://evil.example"},
	} {
		if code := open(header); code != http.StatusForbidden {
			t.Errorf("%v: %d, want 403", header, code)
		}
	}
	// The web app and the apps get as far as the upgrade (a recorder cannot
	// hijack, so the library refuses the plain request, not the caller).
	for _, header := range []map[string]string{
		{"Sec-Fetch-Site": "same-origin", "Origin": "http://tv.local:8477"},
		{"Origin": "http://tv.local:8477"},
		{},
	} {
		if code := open(header); code == http.StatusForbidden {
			t.Errorf("%v refused", header)
		}
	}
}
