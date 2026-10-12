package httpapi

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"broadwave/internal/dvr"
	"broadwave/internal/store"
)

// A skip is matched on the UTC start. An offset must be stored that way, and
// a value that is not a time must be refused instead of saved as text.
func TestSkipStoresTheStartInUTC(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	air := store.Airing{
		ChannelID: 7,
		Title:     "Harbor Watch",
		Subtitle:  "Pilot",
		ProgramID: "HW1",
		Start:     start,
		End:       start.Add(time.Hour),
	}
	if err := st.ReplaceAirings(t.Context(), []store.Airing{air}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(t.Context(), "Harbor Watch", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	before := scheduleItems(t, api)
	if len(before) != 1 || before[0].Skipped || before[0].Airing.Title != "Harbor Watch" {
		t.Fatalf("before %#v", before)
	}

	offset := start.In(time.FixedZone("harbor", -5*3600)).Format(time.RFC3339)
	rec := postJSON(t, api, "/api/v1/schedule/skip", `{"programId":"HW1","title":"Harbor Watch","channelId":7,"start":"`+offset+`"}`)
	skipped := documentedBody(t, doc, "post", "/schedule/skip", "200", rec)
	if skipped["ok"] != true {
		t.Fatalf("skip %#v", skipped)
	}

	key, starts := dvr.SkipParts(air)
	got, err := st.Skips(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !got[key+"|"+starts] {
		t.Fatalf("stored %v, want %s|%s", got, key, starts)
	}

	after := scheduleItems(t, api)
	if len(after) != 1 || !after[0].Skipped || after[0].Reason != "Skipped once" {
		t.Fatalf("after %#v", after)
	}

	for _, body := range []string{`{}`, `{"start":"yesterday"}`} {
		bad := assertErrorBody(t, doc, "post", "/schedule/skip", "400", postJSON(t, api, "/api/v1/schedule/skip", body))
		if bad["code"] != "bad_request" {
			t.Fatalf("%s %#v", body, bad)
		}
	}
}

func scheduleItems(t *testing.T, api http.Handler) []dvr.Planned {
	t.Helper()
	rec := call(t, api, http.MethodGet, "/api/v1/schedule", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("schedule %d %s", rec.Code, rec.Body.String())
	}
	var body struct {
		Items []dvr.Planned `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}
