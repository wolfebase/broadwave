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
		{GuideNumber: "4.1", GuideName: "KDVR-DT"},
		{GuideNumber: "4.2", GuideName: "KDVR2"},
		{GuideNumber: "5.1", GuideName: "KCNC"},
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
	if got["KDVR-DT"] != "FOX" || got["KCNC"] != "CBS" || got["ABC"] != "ABC" || got["KDVR2"] != "" || got["FOX NEWS"] != "" {
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
	if !fav["KDVR-DT"] || !fav["KCNC"] || !fav["ABC"] || fav["KDVR2"] || fav["FOX NEWS"] {
		t.Fatalf("favorites %#v", fav)
	}

	calls := get(t, h, "/api/v1/affiliations")
	var table struct {
		Calls map[string]string `json:"calls"`
	}
	if err := json.Unmarshal(calls.Body.Bytes(), &table); err != nil {
		t.Fatal(err)
	}
	if table.Calls["KUSA"] != "NBC" || table.Calls["KDVR2"] != "" {
		t.Fatalf("table lookup KUSA=%q", table.Calls["KUSA"])
	}
}

func TestPSIPNamesNameTheNetwork(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "FAKE", FriendlyName: "Fake", ModelNumber: "HDHR4-2US", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "5.1", GuideName: "FOX 5"},
		{GuideNumber: "8.1", GuideName: "PBS"},
		{GuideNumber: "14.1", GuideName: "UNIVSN"},
		{GuideNumber: "20.1", GuideName: "CBWT"},
		{GuideNumber: "22.1", GuideName: "GLOBAL"},
		{GuideNumber: "22.2", GuideName: "PBS KIDS"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Assets: fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("<!doctype html>")}}}).Handler()
	var listed struct {
		Channels []store.Channel `json:"channels"`
	}
	if err := json.Unmarshal(get(t, h, "/api/v1/channels?guide=1").Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, ch := range listed.Channels {
		got[ch.GuideNumber] = ch.Network
	}
	want := map[string]string{"5.1": "FOX", "8.1": "PBS", "14.1": "Univision", "20.1": "CBC", "22.1": "Global", "22.2": ""}
	for number, net := range want {
		if got[number] != net {
			t.Fatalf("%s: %q, want %q (%v)", number, got[number], net, got)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/channels/star", bytes.NewBufferString("{}"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var starred struct {
		Starred []struct {
			Network string `json:"network"`
		} `json:"starred"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &starred); err != nil {
		t.Fatal(err)
	}
	nets := map[string]bool{}
	for _, row := range starred.Starred {
		nets[row.Network] = true
	}
	if len(starred.Starred) != 3 || !nets["FOX"] || !nets["CBC"] || !nets["Global"] {
		t.Fatalf("starred %+v", starred.Starred)
	}
}
