package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
