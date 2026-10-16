package httpapi

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
	"broadwave/internal/store"
)

// The cached guide, channel list, and schedule change the moment the data
// they read does, and a repeat with the ETag is a 304.
func TestCachedAnswersFollowTheData(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "AAAA0001", FriendlyName: "FLEX", BaseURL: "http://127.0.0.1:1", TunerCount: 2},
		[]hdhr.Channel{{GuideNumber: "4.1", GuideName: "KNBC"}, {GuideNumber: "5.1", GuideName: "KTLA"}}); err != nil {
		t.Fatal(err)
	}
	chs, _ := st.Channels(ctx, false)
	now := time.Now().UTC().Truncate(time.Minute)
	s := &Server{Store: st}
	h := s.Handler()
	fetch := func(path, etag string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	body := func(rec *httptest.ResponseRecorder) string {
		if rec.Header().Get("Content-Encoding") != "gzip" {
			return rec.Body.String()
		}
		zr, err := gzip.NewReader(bytes.NewReader(rec.Body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(zr)
		return string(out)
	}

	first := fetch("/api/v1/airings", "")
	if first.Code != http.StatusOK || strings.Contains(body(first), "Late News") {
		t.Fatalf("%d %s", first.Code, body(first))
	}
	if again := fetch("/api/v1/airings", first.Header().Get("ETag")); again.Code != http.StatusNotModified {
		t.Fatalf("repeat with the tag: %d", again.Code)
	}
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: chs[0].ID, Title: "Late News", Start: now, End: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	changed := fetch("/api/v1/airings", first.Header().Get("ETag"))
	if changed.Code != http.StatusOK || !strings.Contains(body(changed), "Late News") {
		t.Fatalf("a new listing was not served: %d %s", changed.Code, body(changed))
	}

	list := fetch("/api/v1/channels", "")
	if strings.Contains(body(list), "Renamed") {
		t.Fatal("unexpected name")
	}
	name := "Renamed"
	if _, err := st.PatchChannel(ctx, chs[1].ID, store.ChannelPatch{CustomName: &name}); err != nil {
		t.Fatal(err)
	}
	if after := fetch("/api/v1/channels", ""); !strings.Contains(body(after), "Renamed") {
		t.Fatalf("a renamed channel was not served: %s", body(after))
	}

	plan := fetch("/api/v1/schedule", "")
	if strings.Contains(body(plan), "Late News") {
		t.Fatalf("no pass yet: %s", body(plan))
	}
	if err := st.AddPass(ctx, "Late News", 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	if after := fetch("/api/v1/schedule", ""); !strings.Contains(body(after), "Late News") {
		t.Fatalf("a new pass was not planned: %s", body(after))
	}
}

func TestBodyCacheKeepsTheNewest(t *testing.T) {
	var c bodyCache
	for i := range bodyCacheMax + 5 {
		c.put(string(rune('a'+i)), &cachedBody{etag: "x"})
	}
	if len(c.entries) != bodyCacheMax {
		t.Fatalf("%d entries", len(c.entries))
	}
	if c.get(string(rune('a'+bodyCacheMax+4))) == nil {
		t.Fatal("the newest entry was dropped")
	}
}

func TestGridAiringsLeaveOutDetails(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	now := time.Now().UTC().Truncate(time.Minute)
	if err := st.InsertAirings(ctx, []store.Airing{{ChannelID: 1, Title: "News", Description: "Long words", Cast: "Anchor", ProgramID: "EP1", Start: now, End: now.Add(time.Hour)}}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st}).Handler()
	grid := get(t, h, "/api/v1/airings?fields=grid").Body.String()
	full := get(t, h, "/api/v1/airings").Body.String()
	if strings.Contains(grid, "Long words") || strings.Contains(grid, "EP1") || !strings.Contains(grid, "News") {
		t.Fatalf("grid %s", grid)
	}
	if !strings.Contains(full, "Long words") {
		t.Fatalf("full %s", full)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/airings?fields=all", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("fields=all %d", rec.Code)
	}
}

func TestListsAreCompressed(t *testing.T) {
	h := (&Server{Store: testStore(t)}).Handler()
	for _, path := range []string{"/api/v1/search?q=news", "/api/v1/recordings", "/api/v1/diagnostics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || rec.Header().Get("Content-Encoding") != "gzip" || rec.Header().Get("ETag") == "" {
			t.Fatalf("%s: %d %q", path, rec.Code, rec.Header().Get("Content-Encoding"))
		}
	}
}
