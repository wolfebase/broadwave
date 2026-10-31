package httpapi

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A playlist link or folder that does not open is the most common first
// mistake. The answer says what to check, not "something went wrong".
func TestASourceThatDoesNotOpenSaysWhatToCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closed := ln.Addr().String()
	ln.Close()
	locked := t.TempDir()
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	h := (&Server{Store: testStore(t)}).Handler()
	for body, want := range map[string]string{
		`{"kind":"m3u","url":"http://` + closed + `/playlist.m3u"}`:                           "can't reach that address",
		`{"kind":"m3u","url":"` + filepath.Join(t.TempDir(), "missing.m3u") + `"}`:            "can't read that file",
		`{"kind":"folder","name":"Shelf","url":"` + filepath.Join(t.TempDir(), "gone") + `"}`: "can't read that file",
		`{"kind":"folder","name":"Locked","url":"` + locked + `"}`:                            "PUID and PGID",
	} {
		if strings.Contains(body, "Locked") && os.Geteuid() == 0 {
			continue // root reads any folder
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		var got struct{ Code, Message string }
		_ = json.Unmarshal(rec.Body.Bytes(), &got)
		if rec.Code != http.StatusBadRequest || !strings.Contains(got.Message, want) {
			t.Errorf("%s: %d %s", body, rec.Code, rec.Body.String())
		}
	}
}
