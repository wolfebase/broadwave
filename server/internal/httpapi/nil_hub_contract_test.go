package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestMissingPlayerIsUnavailable(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	unavailable := func(method, url, specPath, body string) {
		t.Helper()
		var rec *httptest.ResponseRecorder
		if method == http.MethodGet {
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(method, url, nil))
		} else {
			rec = sendJSON(t, api, method, url, body)
		}
		got := assertErrorBody(t, doc, method, specPath, "503", rec)
		if got["code"] != "unavailable" || got["message"] != "player is not configured" {
			t.Fatalf("%s %s %#v", method, specPath, got)
		}
	}

	unavailable(http.MethodGet, "/api/v1/backup", "/backup", "")
	unavailable(http.MethodPost, "/api/v1/recordings/1/stop", "/recordings/{id}/stop", "")
	unavailable(http.MethodPost, "/api/v1/recordings", "/recordings", `{"channelId":1}`)

	id, err := st.CreateRecording(t.Context(), store.Recording{
		GuideNumber: "7.1", Title: "Harbor Watch", Status: "complete", StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	unavailable(http.MethodPost, fmt.Sprintf("/api/v1/recordings/%d/detect", id), "/recordings/{id}/detect", "")

	bad := sendJSON(t, api, http.MethodPost, "/api/v1/recordings/nope/stop", "")
	badBody := assertErrorBody(t, doc, http.MethodPost, "/recordings/{id}/stop", "400", bad)
	if badBody["message"] != "invalid recording" {
		t.Fatalf("%#v", badBody)
	}
	missing := sendJSON(t, api, http.MethodPost, "/api/v1/recordings/999/detect", "")
	missBody := assertErrorBody(t, doc, http.MethodPost, "/recordings/{id}/detect", "404", missing)
	if missBody["message"] != "recording not found" {
		t.Fatalf("%#v", missBody)
	}
}
