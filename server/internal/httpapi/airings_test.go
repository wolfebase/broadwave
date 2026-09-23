package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"testing/fstest"
	"time"

	"waveguide/internal/hdhr"
	"waveguide/internal/store"
)

func TestAiringsWindowAndCompression(t *testing.T) {
	st := testStore(t)
	err := st.UpsertDevice(context.Background(), hdhr.Device{
		DeviceID: "10611B4C", FriendlyName: "DUO", ModelNumber: "HDHR5-2US",
		BaseURL: "http://192.168.1.252", TunerCount: 2,
	}, []hdhr.Channel{
		{GuideNumber: "4.1", GuideName: "WDAF", VideoCodec: "MPEG2", AudioCodec: "AC3"},
		{GuideNumber: "5.1", GuideName: "KCTV", VideoCodec: "MPEG2", AudioCodec: "AC3"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	res := get(t, h, "/api/v1/channels?guide=1")
	var lineup struct {
		Channels []store.Channel `json:"channels"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &lineup); err != nil {
		t.Fatal(err)
	}
	if len(lineup.Channels) != 2 {
		t.Fatalf("channels %+v", lineup.Channels)
	}
	now := time.Now().UTC().Truncate(time.Second)
	rows := []store.Airing{
		{ChannelID: lineup.Channels[0].ID, Title: "Now", Start: now, End: now.Add(time.Hour)},
		{ChannelID: lineup.Channels[0].ID, Title: "Later", Start: now.Add(10 * time.Hour), End: now.Add(11 * time.Hour)},
		{ChannelID: lineup.Channels[1].ID, Title: "Other", Start: now.Add(time.Hour), End: now.Add(2 * time.Hour)},
	}
	if err := st.ReplaceAirings(context.Background(), rows); err != nil {
		t.Fatal(err)
	}

	from := now.Add(-time.Hour).Format(time.RFC3339)
	to := now.Add(5 * time.Hour).Format(time.RFC3339)
	window := get(t, h, "/api/v1/airings?from="+from+"&to="+to)
	titles := airingTitles(t, window.Body.Bytes())
	if len(titles) != 2 || titles[0] != "Now" || titles[1] != "Other" {
		t.Fatalf("window %v", titles)
	}

	only := get(t, h, "/api/v1/airings?from="+from+"&to="+to+"&channels="+strconv.FormatInt(lineup.Channels[0].ID, 10))
	titles = airingTitles(t, only.Body.Bytes())
	if len(titles) != 1 || titles[0] != "Now" {
		t.Fatalf("channel filter %v", titles)
	}

	wide := get(t, h, "/api/v1/airings")
	if len(airingTitles(t, wide.Body.Bytes())) != 3 {
		t.Fatalf("full range %v", airingTitles(t, wide.Body.Bytes()))
	}

	bad := httptest.NewRequest(http.MethodGet, "/api/v1/airings?from="+to+"&to="+from, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("backwards window %d", rec.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/airings", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("encoding %q body %s", rec.Header().Get("Content-Encoding"), rec.Body.Bytes())
	}
	zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	if len(airingTitles(t, plain)) != 3 {
		t.Fatalf("gzip body %s", plain)
	}
	if rec.Body.Len() >= len(plain) {
		t.Fatalf("gzip %d not smaller than %d", rec.Body.Len(), len(plain))
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("missing etag")
	}
	again := httptest.NewRequest(http.MethodGet, "/api/v1/airings", nil)
	again.Header.Set("If-None-Match", etag)
	again.Header.Set("Accept-Encoding", "gzip")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, again)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("304 %d %s", rec.Code, rec.Body.String())
	}
}

func TestPrecompressedAsset(t *testing.T) {
	assets := fstest.MapFS{
		"index.html":       &fstest.MapFile{Data: []byte("<!doctype html><title>Waveguide</title>")},
		"assets/app.js":    &fstest.MapFile{Data: []byte("console.log('waveguide')")},
		"assets/app.js.gz": &fstest.MapFile{Data: []byte{0x1f, 0x8b, 0x08, 0x00}},
		"assets/app.js.br": &fstest.MapFile{Data: []byte("brotli-bytes")},
	}
	h := (&Server{Store: testStore(t), Assets: assets}).Handler()
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Header.Set("Accept-Encoding", "gzip, br")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "br" {
		t.Fatalf("br %d %q %s", rec.Code, rec.Header().Get("Content-Encoding"), rec.Body.Bytes())
	}
	if rec.Body.String() != "brotli-bytes" {
		t.Fatalf("body %s", rec.Body.Bytes())
	}
	if rec.Header().Get("Content-Type") != "text/javascript; charset=utf-8" {
		t.Fatalf("type %s", rec.Header().Get("Content-Type"))
	}
}

func airingTitles(t *testing.T, body []byte) []string {
	t.Helper()
	var payload struct {
		Airings []store.Airing `json:"airings"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	out := make([]string, len(payload.Airings))
	for i, row := range payload.Airings {
		out[i] = row.Title
	}
	return out
}
