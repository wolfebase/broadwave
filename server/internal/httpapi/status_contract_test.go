package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// TestDVRErrorsMatchTheSpec starts the API with no tuner and checks the
// recording, pass, team, and library-channel errors the handlers write.
// Saving a schedule fix that changes no pass is also 404; that path needs a
// planned airing, so the test only requires the spec to name it.
func TestDVRErrorsMatchTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	withHub := (&Server{
		Store:   st,
		Hub:     &live.Hub{Store: st},
		Version: "dev",
		Staging: true,
	}).Handler()

	cases := []dvrStatus{
		{"stop bad id", http.MethodPost, "/api/v1/recordings/nope/stop", "/recordings/{id}/stop", "", "400", false},
		{"delete bad id", http.MethodDelete, "/api/v1/recordings/nope", "/recordings/{id}", "", "400", false},
		{"delete missing", http.MethodDelete, "/api/v1/recordings/1", "/recordings/{id}", "", "404", false},
		{"play bad id", http.MethodPost, "/api/v1/recordings/nope/play", "/recordings/{id}/play", "", "400", false},
		{"play missing", http.MethodPost, "/api/v1/recordings/1/play", "/recordings/{id}/play", "", "404", false},
		{"progress bad id", http.MethodPut, "/api/v1/recordings/nope/progress", "/recordings/{id}/progress", `{"position":1}`, "400", false},
		{"progress missing position", http.MethodPut, "/api/v1/recordings/1/progress", "/recordings/{id}/progress", `{}`, "400", false},
		{"progress missing row", http.MethodPut, "/api/v1/recordings/1/progress", "/recordings/{id}/progress", `{"position":1}`, "404", false},
		{"watched bad id", http.MethodPut, "/api/v1/recordings/nope/watched", "/recordings/{id}/watched", `{"watched":true}`, "400", false},
		{"watched missing", http.MethodPut, "/api/v1/recordings/1/watched", "/recordings/{id}/watched", `{"watched":true}`, "404", false},
		{"keep bad id", http.MethodPut, "/api/v1/recordings/nope/keep", "/recordings/{id}/keep", `{"keep":true}`, "400", false},
		{"keep missing", http.MethodPut, "/api/v1/recordings/1/keep", "/recordings/{id}/keep", `{"keep":true}`, "404", false},
		{"move bad id", http.MethodPost, "/api/v1/recordings/nope/move", "/recordings/{id}/move", `{"name":"Show/Ep"}`, "400", false},
		{"move no library", http.MethodPost, "/api/v1/recordings/1/move", "/recordings/{id}/move", `{"name":"Show/Ep"}`, "503", false},
		{"move missing", http.MethodPost, "/api/v1/recordings/999/move", "/recordings/{id}/move", `{"name":"Show/Ep"}`, "404", true},
		{"file bad id", http.MethodGet, "/api/v1/recordings/nope/file", "/recordings/{id}/file", "", "400", false},
		{"file missing", http.MethodGet, "/api/v1/recordings/1/file", "/recordings/{id}/file", "", "404", false},
		{"again bad id", http.MethodGet, "/api/v1/recordings/nope/again", "/recordings/{id}/again", "", "400", false},
		{"again missing", http.MethodGet, "/api/v1/recordings/1/again", "/recordings/{id}/again", "", "404", false},
		{"markers bad id", http.MethodGet, "/api/v1/recordings/nope/markers", "/recordings/{id}/markers", "", "400", false},
		{"marker bad id", http.MethodPost, "/api/v1/recordings/nope/markers", "/recordings/{id}/markers", `{"start":1,"end":2}`, "400", false},
		{"marker empty span", http.MethodPost, "/api/v1/recordings/1/markers", "/recordings/{id}/markers", `{"start":2,"end":1}`, "400", false},
		{"detect bad id", http.MethodPost, "/api/v1/recordings/nope/detect", "/recordings/{id}/detect", "", "400", false},
		{"detect missing", http.MethodPost, "/api/v1/recordings/1/detect", "/recordings/{id}/detect", "", "404", false},
		{"delete marker bad id", http.MethodDelete, "/api/v1/markers/nope", "/markers/{id}", "", "400", false},
		{"delete marker missing", http.MethodDelete, "/api/v1/markers/1", "/markers/{id}", "", "404", false},
		{"skip bad json", http.MethodPost, "/api/v1/schedule/skip", "/schedule/skip", `{`, "400", false},
		{"order bad json", http.MethodPut, "/api/v1/passes/order", "/passes/order", `{`, "400", false},
		{"pass bad id", http.MethodPatch, "/api/v1/passes/nope", "/passes/{id}", `{}`, "400", false},
		{"pass missing", http.MethodPatch, "/api/v1/passes/1", "/passes/{id}", `{}`, "404", false},
		{"delete pass bad id", http.MethodDelete, "/api/v1/passes/nope", "/passes/{id}", "", "400", false},
		{"delete pass missing", http.MethodDelete, "/api/v1/passes/1", "/passes/{id}", "", "404", false},
		{"team needs a name", http.MethodPut, "/api/v1/teams", "/teams", `{}`, "400", false},
		{"team bad id", http.MethodDelete, "/api/v1/teams/nope", "/teams/{id}", "", "400", false},
		{"team missing", http.MethodDelete, "/api/v1/teams/1", "/teams/{id}", "", "404", false},
		{"virtual needs a name", http.MethodPost, "/api/v1/virtuals", "/virtuals", `{}`, "400", false},
		{"virtual bad id", http.MethodPatch, "/api/v1/virtuals/nope", "/virtuals/{id}", `{}`, "400", false},
		{"virtual missing", http.MethodPatch, "/api/v1/virtuals/1", "/virtuals/{id}", `{}`, "404", false},
		{"virtual play bad id", http.MethodPost, "/api/v1/virtuals/nope/play", "/virtuals/{id}/play", `{"index":0}`, "400", false},
		{"virtual play missing", http.MethodPost, "/api/v1/virtuals/1/play", "/virtuals/{id}/play", `{"index":0}`, "404", false},
		{"virtual play no index", http.MethodPost, "/api/v1/virtuals/1/play", "/virtuals/{id}/play", ``, "400", false},
		{"record bad json", http.MethodPost, "/api/v1/recordings", "/recordings", `{`, "400", false},
		{"record missing channel", http.MethodPost, "/api/v1/recordings", "/recordings", `{"channelId":1}`, "404", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := api
			if tc.hub {
				handler = withHub
			}
			checkDVRStatus(t, doc, handler, tc)
		})
	}

	ready, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete",
		StartedAt: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	active, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Watch", Status: "recording",
		StartedAt: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := st.CreateVirtual(t.Context(), "900", "Harbor Watch", []int64{ready})
	if err != nil {
		t.Fatal(err)
	}

	seeded := []dvrStatus{
		{"play without a player", http.MethodPost, fmt.Sprintf("/api/v1/recordings/%d/play", ready), "/recordings/{id}/play", "", "503", false},
		{"delete while recording", http.MethodDelete, fmt.Sprintf("/api/v1/recordings/%d", active), "/recordings/{id}", "", "409", false},
		{"virtual play without a player", http.MethodPost, fmt.Sprintf("/api/v1/virtuals/%d/play", channel.ID), "/virtuals/{id}/play", `{"index":0}`, "503", false},
	}
	for _, tc := range seeded {
		t.Run(tc.name, func(t *testing.T) {
			checkDVRStatus(t, doc, api, tc)
		})
	}

	if _, ok, err := responseSchema(doc, "post", "/schedule/fix", "404"); err != nil || !ok {
		t.Fatalf("schedule fix 404: ok=%v %v", ok, err)
	}
	item, _ := asMap(doc.paths["/recordings"])
	op, _ := asMap(item["post"])
	responses, _ := asMap(op["responses"])
	raw, _ := asMap(responses["503"])
	desc, _ := raw["description"].(string)
	if !strings.Contains(desc, "tuner_silent") {
		t.Fatalf("start recording 503: %q", desc)
	}
}

type dvrStatus struct {
	name, method, url, spec, body, status string
	hub                                   bool
}

func checkDVRStatus(t *testing.T, doc *apiDoc, handler http.Handler, tc dvrStatus) {
	t.Helper()
	req := httptest.NewRequest(tc.method, tc.url, strings.NewReader(tc.body))
	if tc.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := assertErrorBody(t, doc, tc.method, tc.spec, tc.status, rec)
	want := map[string]string{
		"400": "bad_request",
		"404": "not_found",
		"409": "conflict",
		"503": "unavailable",
	}[tc.status]
	if body["code"] != want {
		t.Fatalf("code %#v", body["code"])
	}
}
