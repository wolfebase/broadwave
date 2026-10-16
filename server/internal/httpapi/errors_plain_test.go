package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnknownAPIPathIsAJSON404(t *testing.T) {
	s := &Server{Store: testStore(t)}
	h := s.Handler()
	for _, path := range []string{"/api/v1/nope", "/api/nope/deeper", "/api/v1/"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound || !strings.HasPrefix(rec.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("%s: %d %s", path, rec.Code, rec.Header().Get("Content-Type"))
		}
		var body struct{ Code string }
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Code != "not_found" {
			t.Fatalf("%s: %s", path, rec.Body)
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("health %d", rec.Code)
	}
}

func TestSystemErrorsReadPlain(t *testing.T) {
	disk := fmt.Errorf("save: %w", &fs.PathError{Op: "open", Path: "/config/catalog.db", Err: fs.ErrPermission})
	rec := httptest.NewRecorder()
	writeError(rec, disk)
	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), "/config") || !strings.Contains(rec.Body.String(), "server log") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	writeError(rec, errors.New("no tuner"))
	if !strings.Contains(rec.Body.String(), "no tuner") {
		t.Fatalf("a message the server words itself was replaced: %s", rec.Body)
	}
}
