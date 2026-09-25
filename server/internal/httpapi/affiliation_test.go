package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

func TestStarBigFour(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "FAKE", FriendlyName: "Fake", ModelNumber: "HDHR4-2US", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF-DT"},
		{GuideNumber: "4.2", GuideName: "WDAF2"},
		{GuideNumber: "5.1", GuideName: "KCTV"},
		{GuideNumber: "9.1", GuideName: "ABC"},
		{GuideNumber: "11.1", GuideName: "FOX NEWS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}}).Handler()

	res := get(t, h, "/api/v1/channels?guide=1")
	var listed struct {
		Channels []store.Channel `json:"channels"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ch := range listed.Channels {
		got[ch.GuideName] = ch.Network
	}
	if got["WDAF-DT"] != "FOX" || got["KCTV"] != "CBS" || got["ABC"] != "ABC" || got["WDAF2"] != "" || got["FOX NEWS"] != "" {
		t.Fatalf("networks: %#v", got)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/star", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("star %d %s", rec.Code, rec.Body.String())
	}
	var starred struct {
		Starred []struct {
			GuideName string `json:"guideName"`
			Network   string `json:"network"`
		} `json:"starred"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &starred); err != nil {
		t.Fatal(err)
	}
	if len(starred.Starred) != 3 {
		t.Fatalf("starred %+v", starred.Starred)
	}

	res = get(t, h, "/api/v1/channels?guide=1")
	if err := json.Unmarshal(res.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	fav := map[string]bool{}
	for _, ch := range listed.Channels {
		fav[ch.GuideName] = ch.Favorite
	}
	if !fav["WDAF-DT"] || !fav["KCTV"] || !fav["ABC"] || fav["WDAF2"] || fav["FOX NEWS"] {
		t.Fatalf("favorites %#v", fav)
	}

	calls := get(t, h, "/api/v1/affiliations")
	var table struct {
		Calls map[string]string `json:"calls"`
	}
	if err := json.Unmarshal(calls.Body.Bytes(), &table); err != nil {
		t.Fatal(err)
	}
	if table.Calls["KSHB"] != "NBC" || table.Calls["WDAF2"] != "" {
		t.Fatalf("table lookup KSHB=%q", table.Calls["KSHB"])
	}
}
