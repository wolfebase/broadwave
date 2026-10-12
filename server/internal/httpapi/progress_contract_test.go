package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

// A playhead is seconds from the start. Zero is a real spot. A missing, null,
// or negative position is refused, and the request schema says the same.
func TestSaveProgressMatchesTheSpec(t *testing.T) {
	doc := loadOpenAPI(t)
	schema := requestSchema(t, doc, http.MethodPut, "/recordings/{id}/progress")
	props, _ := asMap(schema["properties"])
	position, ok := asMap(props["position"])
	if !ok {
		t.Fatal("position is not a schema")
	}
	min, ok := asNumber(position["minimum"])
	if !ok || min != 0 {
		t.Fatalf("position minimum = %#v, want 0", position["minimum"])
	}
	if err := validateSchema(doc, schema, jsonValue(t, `{"position":0}`), "$", 0); err != nil {
		t.Fatalf("spec rejects zero: %v", err)
	}
	if err := validateSchema(doc, schema, jsonValue(t, `{"position":12.5}`), "$", 0); err != nil {
		t.Fatalf("spec rejects 12.5: %v", err)
	}
	for _, raw := range []string{`{"position":-1}`, `{"position":-0.5}`} {
		err := validateSchema(doc, schema, jsonValue(t, raw), "$", 0)
		if err == nil || !strings.Contains(err.Error(), "under") {
			t.Fatalf("spec accepts %s: %v", raw, err)
		}
	}
	for _, raw := range []string{`{}`, `{"position":null}`} {
		if err := validateSchema(doc, schema, jsonValue(t, raw), "$", 0); err == nil {
			t.Fatalf("spec accepts %s", raw)
		}
	}

	st := testStore(t)
	id, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete",
		StartedAt: time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	url := fmt.Sprintf("/api/v1/recordings/%d/progress", id)
	put := func(body string) *httptest.ResponseRecorder {
		return sendJSON(t, api, http.MethodPut, url, body)
	}

	for _, body := range []string{`{}`, `{"position":null}`, `{"position":-1}`, `{"position":-0.5}`} {
		got := assertErrorBody(t, doc, http.MethodPut, "/recordings/{id}/progress", "400", put(body))
		if got["code"] != "bad_request" || got["message"] != "position required" {
			t.Fatalf("%s %#v", body, got)
		}
	}
	row, err := st.Recording(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.ProgressAt != nil {
		t.Fatal("a refused position was stored")
	}

	kept := documentedBody(t, doc, http.MethodPut, "/recordings/{id}/progress", "200", put(`{"position":4.5}`))
	if kept["position"] != 4.5 {
		t.Fatalf("stored response %#v", kept["position"])
	}
	refused := assertErrorBody(t, doc, http.MethodPut, "/recordings/{id}/progress", "400", put(`{"position":-1}`))
	if refused["message"] != "position required" {
		t.Fatalf("negative after a save: %#v", refused["message"])
	}
	if seconds, err := st.Progress(t.Context(), id); err != nil || seconds != 4.5 {
		t.Fatalf("negative overwrote the playhead: %v %v", seconds, err)
	}

	saved := documentedBody(t, doc, http.MethodPut, "/recordings/{id}/progress", "200", put(`{"position":0}`))
	if saved["position"] != float64(0) {
		t.Fatalf("zero response %#v", saved["position"])
	}
	row, err = st.Recording(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if row.Position != 0 || row.ProgressAt == nil {
		t.Fatalf("zero not stored: position %v at %v", row.Position, row.ProgressAt)
	}
	if seconds, err := st.Progress(t.Context(), id); err != nil || seconds != 0 {
		t.Fatalf("progress %v %v", seconds, err)
	}
}
