package httpapi

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"broadwave/internal/store"
)

// A library-channel patch changes only the fields it sends.
func TestOmittedVirtualFieldsStay(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	started := time.Date(2026, 9, 24, 15, 0, 0, 0, time.UTC)
	watch, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Watch", Status: "complete", StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	lights, err := st.CreateRecording(t.Context(), store.Recording{
		ChannelID: 1, GuideNumber: "12.1", Title: "Harbor Lights", Status: "complete", StartedAt: started,
	})
	if err != nil {
		t.Fatal(err)
	}
	channel, err := st.CreateVirtual(t.Context(), "900", "Harbor Channel", []int64{watch, lights})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateVirtual(t.Context(), channel.ID, "custom", "Harbor Watch", nil); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	path := fmt.Sprintf("/api/v1/virtuals/%d", channel.ID)
	patch := func(body string) map[string]any {
		t.Helper()
		return documentedBody(t, doc, http.MethodPatch, "/virtuals/{id}", "200", sendJSON(t, api, http.MethodPatch, path, body))
	}

	kept := patch(`{"orderMode":"season"}`)
	if kept["ruleTitle"] != "Harbor Watch" || kept["orderMode"] != "season" {
		t.Fatalf("order patch %#v", kept)
	}
	if ids := numberList(t, kept["recordings"]); len(ids) != 2 || ids[0] != float64(watch) || ids[1] != float64(lights) {
		t.Fatalf("recordings %#v", kept["recordings"])
	}

	cleared := patch(`{"recordings":[]}`)
	if cleared["ruleTitle"] != "Harbor Watch" || cleared["orderMode"] != "season" {
		t.Fatalf("after a cleared list %#v", cleared)
	}
	if ids := numberList(t, cleared["recordings"]); len(ids) != 0 {
		t.Fatalf("cleared recordings %#v", cleared["recordings"])
	}

	again := patch(`{"orderMode":"custom"}`)
	if again["ruleTitle"] != "Harbor Watch" || len(numberList(t, again["recordings"])) != 0 {
		t.Fatalf("second order patch %#v", again)
	}
}

func numberList(t *testing.T, value any) []any {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("list is %T", value)
	}
	return list
}
