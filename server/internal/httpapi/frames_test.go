package httpapi

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
)

func TestFrameIsStaleAfterTenMinutes(t *testing.T) {
	dir := t.TempDir()
	path := live.FramePath(dir, 7, 480)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-11 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	wide := live.FramePath(dir, 7, 1280)
	if err := os.WriteFile(wide, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	stale := get(t, h, "/api/v1/channels/7/frame")
	if stale.Code != 200 || stale.Header().Get("X-Frame-Stale") != "1" || stale.Header().Get("Last-Modified") == "" {
		t.Fatalf("stale %d %v", stale.Code, stale.Header())
	}
	fresh := get(t, h, "/api/v1/channels/7/frame?w=1280")
	if fresh.Code != 200 || fresh.Header().Get("X-Frame-Stale") != "" {
		t.Fatalf("fresh %d %q", fresh.Code, fresh.Header().Get("X-Frame-Stale"))
	}
	missingReq := httptest.NewRequest(http.MethodGet, "/api/v1/channels/9/frame", nil)
	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, missingReq)
	if missing.Code != 404 {
		t.Fatalf("missing %d", missing.Code)
	}
}
