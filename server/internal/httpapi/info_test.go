package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// serverInfoDoc is what a client decodes from GET /api/v1/server.
// minAppVersion is empty when a previous release omitted it.
type serverInfoDoc struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Version       string   `json:"version"`
	APIVersion    int      `json:"apiVersion"`
	Features      []string `json:"features"`
	MinAppVersion string   `json:"minAppVersion"`
}

func TestOlderServerFixtureDecodes(t *testing.T) {
	// Previous release: apiVersion 1, no minAppVersion, and wholeHomeSync not listed yet.
	const older = `{"id":"s","name":"Home","version":"0.9.0","apiVersion":1,"features":["live","renditions","events","dvr","passes","virtualChannels","commercialDetection","hdhrEmulation","export"]}`
	var decoded serverInfoDoc
	if err := json.Unmarshal([]byte(older), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.APIVersion != 1 || decoded.MinAppVersion != "" || decoded.Version != "0.9.0" || decoded.Name != "Home" {
		t.Fatalf("%+v", decoded)
	}
	if slices.Contains(decoded.Features, "wholeHomeSync") {
		t.Fatal("older fixture should not list wholeHomeSync")
	}
	if !slices.Contains(decoded.Features, "live") || !slices.Contains(decoded.Features, "dvr") {
		t.Fatalf("%+v", decoded)
	}
}

func TestServerInfoReportsMinAppVersion(t *testing.T) {
	st := testStore(t)
	h := (&Server{Store: st, Version: "dev"}).Handler()
	res := get(t, h, "/api/v1/server")
	var live serverInfoDoc
	if err := json.Unmarshal(res.Body.Bytes(), &live); err != nil {
		t.Fatal(err)
	}
	if live.MinAppVersion != minAppVersion || live.APIVersion != apiVersion {
		t.Fatalf("%s", res.Body.String())
	}
	if !slices.Contains(live.Features, "wholeHomeSync") || !slices.Contains(live.Features, "hdhrEmulation") {
		t.Fatalf("%s", res.Body.String())
	}

	// A client version header is not an access check. Missing or older is still 200.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/server", nil)
	req.Header.Set("X-Broadwave-Client", "0.9")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("older client %d %s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"minAppVersion":"`+minAppVersion+`"`)) {
		t.Fatalf("%s", rec.Body.String())
	}
}

func TestServerInfoDiscoveryKey(t *testing.T) {
	st := testStore(t)
	plain := (&Server{Store: st, Version: "dev"}).Handler()
	res := get(t, plain, "/api/v1/server")
	if bytes.Contains(res.Body.Bytes(), []byte("discoveryKey")) {
		t.Fatalf("empty key was published: %s", res.Body.String())
	}
	const key = "q83vEjRWeJ4q4q4q4q4q4q4q4q4q4q4q4q4q4q4="
	with := (&Server{Store: st, Version: "dev", DiscoveryKey: key}).Handler()
	res = get(t, with, "/api/v1/server")
	if !bytes.Contains(res.Body.Bytes(), []byte(`"discoveryKey":"`+key+`"`)) {
		t.Fatalf("%s", res.Body.String())
	}
}
