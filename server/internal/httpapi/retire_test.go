package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"broadwave/internal/hdhr"
)

func TestRetiredTunerIsNotOffline(t *testing.T) {
	ctx := context.Background()
	st := testStore(t)
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "DUO1", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:1", TunerCount: 2}, nil); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Clock: func() time.Time { return time.Now().Add(time.Hour) }}
	h := s.Handler()
	patch := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPatch, "/api/v1/devices/"+id, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	if before := get(t, h, "/api/v1/devices").Body.String(); !strings.Contains(before, `"offline":true`) {
		t.Fatalf("a quiet tuner was not offline: %s", before)
	}
	rec := patch("DUO1", `{"retired":true}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"retired":true`) || !strings.Contains(rec.Body.String(), `"offline":false`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	if rec := patch("NOPE", `{"retired":true}`); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown device %d", rec.Code)
	}
	if rec := patch("DUO1", `{}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("no field %d", rec.Code)
	}
	// It answers discovery again: no longer retired.
	if err := st.UpsertDevice(ctx, hdhr.Device{DeviceID: "DUO1", FriendlyName: "DUO", BaseURL: "http://127.0.0.1:1", TunerCount: 2}, nil); err != nil {
		t.Fatal(err)
	}
	if after := get(t, h, "/api/v1/devices").Body.String(); strings.Contains(after, `"retired":true`) {
		t.Fatalf("still retired: %s", after)
	}
}
