package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

// A row left "recording" with nothing writing it (a restart that did not
// resume it) is closed by Stop, keeping its file, and can then be deleted.
func TestStopClosesARecordingNothingIsWriting(t *testing.T) {
	st := testStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	hub := &live.Hub{Store: st, Dir: dir}
	src := filepath.Join(hub.Recordings(), "news.ts")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte{0x47, 1, 2, 3}, 47)
	if err := os.WriteFile(src, body, 0o644); err != nil {
		t.Fatal(err)
	}
	ends := time.Now().Add(time.Hour)
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", GuideNumber: "4.1", Status: "recording",
		Path: src, StartedAt: time.Now().Add(-10 * time.Minute), EndsAt: &ends})
	if err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: st, Hub: hub}).Handler()
	do := func(method, path string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
		return rec
	}
	base := "/api/v1/recordings/" + strconv.FormatInt(id, 10)
	if res := do(http.MethodDelete, base); res.Code != http.StatusConflict {
		t.Fatalf("delete while recording: %d", res.Code)
	}
	if res := do(http.MethodPost, base+"/stop"); res.Code != http.StatusOK {
		t.Fatalf("stop: %d %s", res.Code, res.Body.String())
	}
	rec, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Status != "interrupted" || !rec.Partial || rec.EndedAt == nil || rec.Error == "" {
		t.Fatalf("after stop: %+v", rec)
	}
	if got, err := os.ReadFile(src); err != nil || !bytes.Equal(got, body) {
		t.Fatalf("stop touched the file: %v", err)
	}
	// A second stop leaves the closed row as it is.
	if res := do(http.MethodPost, base+"/stop"); res.Code != http.StatusOK {
		t.Fatalf("second stop: %d", res.Code)
	}
	if again, _ := st.Recording(ctx, id); again.Status != "interrupted" || !again.EndedAt.Equal(*rec.EndedAt) {
		t.Fatalf("second stop changed the row: %+v", again)
	}
	if res := do(http.MethodDelete, base); res.Code != http.StatusOK && res.Code != http.StatusNoContent {
		t.Fatalf("delete after stop: %d %s", res.Code, res.Body.String())
	}
	if _, err := st.Recording(ctx, id); err == nil {
		t.Fatal("the row is still there")
	}
}
