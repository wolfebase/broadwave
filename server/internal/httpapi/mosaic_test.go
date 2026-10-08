package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/live"
)

func TestMosaicRefusesWhatItCannotBuild(t *testing.T) {
	h := (&Server{Store: testStore(t), Hub: &live.Hub{Dir: t.TempDir()}}).Handler()
	for _, body := range []string{`{"channelIds":[1]}`, `{"channelIds":[1,1]}`, `{"channelIds":[1,2,3,4,5]}`, `{"channelIds":[0,2]}`} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/v1/mosaic", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "Pick 2 to 4 different channels.") {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body)
		}
	}
	// The media and export routes start nothing, and a key is only digits and dashes.
	for _, path := range []string{
		"/media/mosaic/1-2/index.m3u8",
		"/media/mosaic/1-2/init.mp4",
		"/media/mosaic/1/index.m3u8",
		"/media/mosaic/1-2/..%2f..%2fbroadwave.db",
		"/media/mosaic/..%2f1-2/init.mp4",
		"/export/mosaic/1",
		"/export/mosaic/a-b",
	} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: %d %s", path, rec.Code, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/mosaic/1-2/stop", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("stop of a mosaic that is not running: %d", rec.Code)
	}
}
