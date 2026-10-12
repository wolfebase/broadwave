package httpapi

import (
	"net/http"
	"testing"
	"time"

	"broadwave/internal/store"
)

// hours is 1 to 168 when to is omitted. A value outside that used to be
// ignored, and the default 48-hour window came back as 200.
func TestAiringsHoursIsRefusedOutsideTheRange(t *testing.T) {
	doc := loadOpenAPI(t)
	st := testStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	rows := []store.Airing{
		{ChannelID: 7, Title: "Harbor Watch", Start: now.Add(20 * time.Minute), End: now.Add(50 * time.Minute)},
		{ChannelID: 7, Title: "Harbor Lights", Start: now.Add(2 * time.Hour), End: now.Add(3 * time.Hour)},
	}
	if err := st.ReplaceAirings(t.Context(), rows); err != nil {
		t.Fatal(err)
	}
	api := (&Server{Store: st, Version: "dev", Staging: true}).Handler()

	one := call(t, api, http.MethodGet, "/api/v1/airings?hours=1", "", "")
	documentedBody(t, doc, "get", "/airings", "200", one)
	if titles := airingTitles(t, one.Body.Bytes()); len(titles) != 1 || titles[0] != "Harbor Watch" {
		t.Fatalf("hours=1 %v", titles)
	}

	three := call(t, api, http.MethodGet, "/api/v1/airings?hours=3", "", "")
	documentedBody(t, doc, "get", "/airings", "200", three)
	if titles := airingTitles(t, three.Body.Bytes()); len(titles) != 2 || titles[0] != "Harbor Watch" || titles[1] != "Harbor Lights" {
		t.Fatalf("hours=3 %v", titles)
	}

	to := now.Add(4 * time.Hour).Format(time.RFC3339)
	kept := call(t, api, http.MethodGet, "/api/v1/airings?to="+to+"&hours=nope", "", "")
	documentedBody(t, doc, "get", "/airings", "200", kept)
	if titles := airingTitles(t, kept.Body.Bytes()); len(titles) != 2 {
		t.Fatalf("to ignores hours %v", titles)
	}

	for _, query := range []string{"hours=0", "hours=999", "hours=nope"} {
		bad := assertErrorBody(t, doc, "get", "/airings", "400", call(t, api, http.MethodGet, "/api/v1/airings?"+query, "", ""))
		if bad["code"] != "bad_request" || bad["message"] != "hours needs to be from 1 to 168" {
			t.Fatalf("%s %#v", query, bad)
		}
	}
}
