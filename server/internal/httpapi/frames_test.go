package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"broadwave/internal/live"
)

func writePreview(t *testing.T, dir string, id int64, width int, age time.Duration) {
	t.Helper()
	path := live.FramePath(dir, id, width)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte{0xff, 0xd8, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if age > 0 {
		when := time.Now().Add(-age)
		if err := os.Chtimes(path, when, when); err != nil {
			t.Fatal(err)
		}
	}
}

func listedFrames(t *testing.T, h http.Handler) []int64 {
	t.Helper()
	res := get(t, h, "/api/v1/frames")
	if res.Code != http.StatusOK {
		t.Fatalf("frames %d %s", res.Code, res.Body.String())
	}
	var body struct {
		Channels []int64 `json:"channels"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Channels == nil {
		t.Fatal("channels is null")
	}
	return body.Channels
}

func TestFramesListsAFreshPreview(t *testing.T) {
	dir := t.TempDir()
	writePreview(t, dir, 3, 480, 0)
	writePreview(t, dir, 7, 480, 0)
	writePreview(t, dir, 8, 480, 11*time.Minute)
	writePreview(t, dir, 11, 1280, 0)
	part := live.FramePath(dir, 12, 480) + ".part"
	if err := os.WriteFile(part, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	got := listedFrames(t, h)
	want := []int64{3, 7}
	if len(got) != len(want) || got[0] != 3 || got[1] != 7 {
		t.Fatalf("frames %v, want %v", got, want)
	}

	emptyDir := t.TempDir()
	none := listedFrames(t, (&Server{Hub: &live.Hub{Dir: emptyDir}}).Handler())
	if len(none) != 0 {
		t.Fatalf("empty dir %v", none)
	}
	missing := listedFrames(t, (&Server{}).Handler())
	if len(missing) != 0 {
		t.Fatalf("no hub %v", missing)
	}
}

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

func TestAFullScreenStillAndItsStamp(t *testing.T) {
	dir := t.TempDir()
	writePreview(t, dir, 7, 480, 0)
	writePreview(t, dir, 7, 1280, 0)
	full := live.FramePath(dir, 7, 1920)
	if err := os.WriteFile(full, []byte{0xff, 0xd8, 0x19, 0x20, 0xff, 0xd9}, 0o644); err != nil {
		t.Fatal(err)
	}
	writePreview(t, dir, 9, 480, 0)
	writePreview(t, dir, 9, 1280, 0)
	h := (&Server{Hub: &live.Hub{Dir: dir}}).Handler()
	if got := get(t, h, "/api/v1/channels/7/frame?w=1920"); got.Code != 200 || got.Body.Len() != 6 {
		t.Fatalf("1920: %d, %d bytes", got.Code, got.Body.Len())
	}
	// A grab from before the full-screen size answers with the wide one.
	if got := get(t, h, "/api/v1/channels/9/frame?w=1920"); got.Code != 200 || got.Body.Len() != 4 {
		t.Fatalf("1920 without one: %d", got.Code)
	}
	res := get(t, h, "/api/v1/frames")
	var body struct {
		Channels []int64 `json:"channels"`
		Frames   []struct {
			ID int64 `json:"id"`
			At int64 `json:"at"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(live.FramePath(dir, 7, 480))
	if err != nil {
		t.Fatal(err)
	}
	if len(body.Frames) != 2 || body.Frames[0].ID != 7 || body.Frames[0].At != info.ModTime().Unix() || len(body.Channels) != 2 {
		t.Fatalf("frames %s", res.Body.String())
	}
	at := strconv.FormatInt(body.Frames[0].At, 10)
	if c := get(t, h, "/api/v1/channels/7/frame?w=1920&at="+at).Header().Get("Cache-Control"); !strings.Contains(c, "immutable") {
		t.Fatalf("a stamped still is not cached: %q", c)
	}
	if c := get(t, h, "/api/v1/channels/7/frame?w=1920&at=12").Header().Get("Cache-Control"); c != "no-cache" {
		t.Fatalf("an old stamp is cached: %q", c)
	}
}
