package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/live"
)

// TestHandlerErrorsMatchTheSpec starts the API with no tuner and checks the
// live, storage, pairing, and guide errors the handlers write. A warm that
// loses a tune, an expired pairing code, and a scoreboard provider that
// fails are named in the spec; those paths need a tuner or a seeded code.
func TestHandlerErrorsMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	withHub := (&Server{
		Store:   st,
		Hub:     &live.Hub{Store: st},
		Version: "dev",
		Staging: true,
	}).Handler()
	noStore := (&Server{Version: "dev", Staging: true}).Handler()

	cases := []handlerStatus{
		{"socket without updates", api, http.MethodGet, "/api/v1/ws", "/ws", "", "", "503"},
		{"remote without an action", api, http.MethodPost, "/api/v1/screens/tv/remote", "/screens/{id}/remote", `{}`, "", "400"},
		{"remote without updates", api, http.MethodPost, "/api/v1/screens/tv/remote", "/screens/{id}/remote", `{"action":"pause"}`, "", "503"},
		{"discover bad type", api, http.MethodPost, "/api/v1/sources/discover", "/sources/discover", "no", "text/plain", "415"},
		{"discover bad json", api, http.MethodPost, "/api/v1/sources/discover", "/sources/discover", `{`, "", "400"},
		{"channel bad id", api, http.MethodPatch, "/api/v1/channels/nope", "/channels/{id}", `{"favorite":true}`, "", "400"},
		{"channel bad json", api, http.MethodPatch, "/api/v1/channels/1", "/channels/{id}", `{`, "", "400"},
		{"channel missing", api, http.MethodPatch, "/api/v1/channels/1", "/channels/{id}", `{"favorite":true}`, "", "404"},
		{"airings bad from", api, http.MethodGet, "/api/v1/airings?from=nope", "/airings", "", "", "400"},
		{"scores off", api, http.MethodGet, "/api/v1/sports/scoreboard", "/sports/scoreboard", "", "", "503"},
		{"watch without live tv", api, http.MethodPost, "/api/v1/watch", "/watch", `{"channelId":1}`, "", "503"},
		{"watch bad json", withHub, http.MethodPost, "/api/v1/watch", "/watch", `{`, "", "400"},
		{"stop bad id", api, http.MethodPost, "/api/v1/watch/nope/stop", "/watch/{id}/stop", "", "", "400"},
		{"warm bad id", api, http.MethodPost, "/api/v1/watch/nope/warm", "/watch/{id}/warm", `{`, "", "400"},
		{"warm bad json", withHub, http.MethodPost, "/api/v1/watch/1/warm", "/watch/{id}/warm", `{`, "", "400"},
		{"plan without a catalog", noStore, http.MethodPost, "/api/v1/multiview/plan", "/multiview/plan", `{"channelIds":[1]}`, "", "503"},
		{"storage without a library", api, http.MethodGet, "/api/v1/storage", "/storage", "", "", "503"},
		{"shows without a library", api, http.MethodGet, "/api/v1/storage/shows", "/storage/shows", "", "", "503"},
		{"restore without a library", api, http.MethodPost, "/api/v1/backup", "/backup", "", "", "503"},
		{"setup bad type", api, http.MethodPost, "/api/v1/setup/finish", "/setup/finish", "no", "text/plain", "415"},
		{"pair code bad json", api, http.MethodPost, "/api/v1/pair/code", "/pair/code", `{`, "", "400"},
		{"pair approve bad json", api, http.MethodPost, "/api/v1/pair/approve", "/pair/approve", `{`, "", "400"},
		{"pair claim bad json", api, http.MethodPost, "/api/v1/pair/claim", "/pair/claim", `{`, "", "400"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			checkHandlerStatus(t, doc, tc)
		})
	}

	for _, path := range []string{"/channels", "/settings", "/airings"} {
		t.Run("cached "+path, func(t *testing.T) {
			checkCached(t, doc, api, path)
		})
	}

	for _, item := range []struct{ method, path, status string }{
		{"get", "/ws", "403"},
		{"post", "/watch/{id}/warm", "409"},
		{"post", "/watch/{id}/warm", "503"},
		{"post", "/pair/approve", "410"},
		{"post", "/pair/claim", "410"},
		{"get", "/sports/scoreboard", "502"},
	} {
		if _, ok, err := responseSchema(doc, item.method, item.path, item.status); err != nil || !ok {
			t.Errorf("%s %s %s: ok=%v %v", item.method, item.path, item.status, ok, err)
		}
	}
	score, _ := asMap(doc.paths["/sports/scoreboard"])
	op, _ := asMap(score["get"])
	responses, _ := asMap(op["responses"])
	if _, ok := responses["500"]; ok {
		t.Fatal("scoreboard still documents 500")
	}
}

type handlerStatus struct {
	name        string
	handler     http.Handler
	method      string
	url         string
	spec        string
	body        string
	contentType string
	status      string
}

func checkHandlerStatus(t *testing.T, doc *apiDoc, tc handlerStatus) {
	t.Helper()
	req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
	switch {
	case tc.contentType != "":
		req.Header.Set("Content-Type", tc.contentType)
	case tc.body != "":
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	tc.handler.ServeHTTP(rec, req)
	body := assertErrorBody(t, doc, tc.method, tc.spec, tc.status, rec)
	want := map[string]string{
		"400": "bad_request",
		"404": "not_found",
		"415": "error",
		"503": "unavailable",
	}[tc.status]
	if body["code"] != want {
		t.Fatalf("code %#v", body["code"])
	}
}

func checkCached(t *testing.T, doc *apiDoc, handler http.Handler, path string) {
	t.Helper()
	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first %s: %d %s", path, first.Code, first.Body.String())
	}
	etag := first.Header().Get("ETag")
	if etag == "" || first.Header().Get("Cache-Control") == "" {
		t.Fatalf("%s cache headers etag=%q control=%q", path, etag, first.Header().Get("Cache-Control"))
	}
	again := httptest.NewRequest(http.MethodGet, "/api/v1"+path, nil)
	again.Header.Set("If-None-Match", etag)
	second := httptest.NewRecorder()
	handler.ServeHTTP(second, again)
	if second.Code != http.StatusNotModified || second.Body.Len() != 0 {
		t.Fatalf("second %s: %d %s", path, second.Code, second.Body.String())
	}
	item, _ := asMap(doc.paths[path])
	op, _ := asMap(item["get"])
	responses, _ := asMap(op["responses"])
	if _, ok := responses["304"]; !ok {
		t.Fatalf("%s does not document 304", path)
	}
	okResp, _ := asMap(responses["200"])
	headers, _ := asMap(okResp["headers"])
	if _, ok := headers["ETag"]; !ok {
		t.Fatalf("%s 200 does not document ETag", path)
	}
	if _, ok := headers["Cache-Control"]; !ok {
		t.Fatalf("%s 200 does not document Cache-Control", path)
	}
}
