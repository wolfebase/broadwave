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

// A skip can be taken back, and records under the rule that wanted the airing.
func TestUnskipPutsTheAiringBackOnThePlan(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	air := store.Airing{ChannelID: 7, Title: "Harbor Watch", Subtitle: "Pilot", ProgramID: "HW1", Start: start, End: start.Add(time.Hour)}
	if err := st.ReplaceAirings(t.Context(), []store.Airing{air}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddPass(t.Context(), "Harbor Watch", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	body := `{"programId":"HW1","title":"Harbor Watch","subtitle":"Pilot","channelId":7,"start":"` + start.In(time.FixedZone("harbor", -5*3600)).Format(time.RFC3339) + `"}`
	postJSON(t, api, "/api/v1/schedule/skip", body)
	if after := scheduleItems(t, api); len(after) != 1 || !after[0].Skipped {
		t.Fatalf("skip didn't take %#v", after)
	}

	back := documentedBody(t, doc, "post", "/schedule/unskip", "200", postJSON(t, api, "/api/v1/schedule/unskip", body))
	if back["ok"] != true {
		t.Fatalf("unskip %#v", back)
	}
	if after := scheduleItems(t, api); len(after) != 1 || after[0].Skipped {
		t.Fatalf("still skipped %#v", after)
	}
	// Nothing left to take back is still fine.
	documentedBody(t, doc, "post", "/schedule/unskip", "200", postJSON(t, api, "/api/v1/schedule/unskip", body))

	for _, bad := range []string{`{}`, `{"start":"yesterday"}`} {
		refused := assertErrorBody(t, doc, "post", "/schedule/unskip", "400", postJSON(t, api, "/api/v1/schedule/unskip", bad))
		if refused["code"] != "bad_request" {
			t.Fatalf("%s %#v", bad, refused)
		}
	}
}

// A timed recording's slot was skipped by its pass, so it comes back the same way.
func TestUnskipATimedRecordingsSlot(t *testing.T) {
	st := testStore(t)
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	if err := st.SkipAiring(t.Context(), dvr.ManualSkipKey(5000), start.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	pass := store.Pass{Title: "Timed", Kind: "manual", ChannelID: 7}
	id, err := st.AddManualPass(t.Context(), pass)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SkipAiring(t.Context(), dvr.ManualSkipKey(id), start.Format(time.RFC3339)); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"passId": id, "title": "Timed", "channelId": 7, "start": start.Format(time.RFC3339)})
	if rec := postJSON(t, api, "/api/v1/schedule/unskip", string(body)); rec.Code != http.StatusOK {
		t.Fatalf("unskip %d %s", rec.Code, rec.Body.String())
	}
	got, err := st.Skips(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if got[dvr.ManualSkipKey(id)+"|"+start.Format(time.RFC3339)] {
		t.Fatalf("slot still skipped %v", got)
	}
	if !got[dvr.ManualSkipKey(5000)+"|"+start.Format(time.RFC3339)] {
		t.Fatalf("another rule's slot went too %v", got)
	}
}

// Record on a skipped airing means record it. The skip used to win silently,
// so the game was lost while its one-time rule said otherwise.
func TestRecordingOneAiringTakesBackItsSkip(t *testing.T) {
	st := testStore(t)
	start := time.Now().UTC().Add(2 * time.Hour).Truncate(time.Second)
	game := store.Airing{ChannelID: 7, Title: "Comets at Otters", ProgramID: "G1", Start: start, End: start.Add(time.Hour)}
	other := store.Airing{ChannelID: 9, Title: "Comets at Otters", ProgramID: "G2", Start: start, End: start.Add(time.Hour)}
	if err := st.ReplaceAirings(t.Context(), []store.Airing{game, other}); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()
	at := start.Format(time.RFC3339)
	for _, air := range []store.Airing{game, other} {
		key, starts := dvr.SkipParts(air)
		if err := st.SkipAiring(t.Context(), key, starts); err != nil {
			t.Fatal(err)
		}
	}
	if rec := postJSON(t, api, "/api/v1/passes", `{"title":"Comets at Otters","channelId":7,"airingStart":"`+at+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("record %d %s", rec.Code, rec.Body.String())
	}
	items := scheduleItems(t, api)
	if len(items) != 1 || items[0].Skipped || items[0].Airing.ChannelID != 7 {
		t.Fatalf("it should record %#v", items)
	}
	got, _ := st.Skips(t.Context())
	key, starts := dvr.SkipParts(other)
	if !got[key+"|"+starts] {
		t.Fatalf("a skip on another channel went too %v", got)
	}
}
