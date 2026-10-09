package live

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/fetchguard"
)

// A source that answers with an error, or not at all, is ErrStreamDown, and
// the HDHomeRun error code stays readable for the busy check.
func TestOpenStreamNamesADeadSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/busy" {
			w.Header().Set("X-HDHomeRun-Error", "805 All Tuners In Use")
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	_, err := openStream(srv.URL+"/live.ts", "", "")
	if !errors.Is(err, ErrStreamDown) {
		t.Fatalf("503: got %v", err)
	}
	_, err = openStream(srv.URL+"/busy", "", "")
	if !errors.Is(err, ErrStreamDown) || !strings.Contains(err.Error(), "805") {
		t.Fatalf("805: got %v", err)
	}
	srv.Close()
	_, err = openStream(srv.URL+"/live.ts", "", "")
	if !errors.Is(err, ErrStreamDown) {
		t.Fatalf("closed: got %v", err)
	}
}

func TestOpenStreamRefusesMetadata(t *testing.T) {
	_, err := openStream("http://169.254.169.254/latest/meta-data", "agent\r\nX-Evil: 1", "")
	if !errors.Is(err, ErrStreamDown) || !errors.Is(err, fetchguard.ErrRefused) {
		t.Fatal(err)
	}
	if strings.Contains(err.Error(), "169.254") {
		t.Fatal(err)
	}
}
