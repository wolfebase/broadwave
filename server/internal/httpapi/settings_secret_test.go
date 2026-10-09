package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGuideAddressRoundTripKeepsThePassword(t *testing.T) {
	st := testStore(t)
	const password = "guide-fixture-password"
	raw := "http://listing.example/xmltv.xml?password=" + password
	if err := st.PutSettings(context.Background(), map[string]string{"guideUrl": raw}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/settings")
	if strings.Contains(res.Body.String(), password) {
		t.Fatalf("settings %s", res.Body.String())
	}
	var got map[string]string
	if err := json.Unmarshal(res.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	shown := got["guideUrl"]
	if shown == "" || shown == raw || strings.Contains(shown, password) {
		t.Fatalf("shown %q", shown)
	}
	put := func(fields map[string]string) *httptest.ResponseRecorder {
		payload, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", bytes.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if rec := put(map[string]string{"guideUrl": shown, "layout": "tv"}); rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), password) {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	stored, err := st.Settings(context.Background())
	if err != nil || stored["guideUrl"] != raw || stored["layout"] != "tv" {
		t.Fatalf("stored %+v %v", stored, err)
	}
	next := "http://listing.example/other.xml"
	if rec := put(map[string]string{"guideUrl": next}); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), next) {
		t.Fatalf("replace %d %s", rec.Code, rec.Body.String())
	}
	stored, err = st.Settings(context.Background())
	if err != nil || stored["guideUrl"] != next {
		t.Fatalf("replaced %+v %v", stored, err)
	}
}
