package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"broadwave/internal/live"
)

// A dead source reaches the player as a code and plain words, not an HTTP status.
func TestAStreamThatIsDownHasItsOwnCode(t *testing.T) {
	rec := httptest.NewRecorder()
	writeError(rec, fmt.Errorf("%w (stream returned 503 Service Unavailable )", live.ErrStreamDown))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	var body struct{ Code, Message string }
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "stream_down" || body.Message != live.ErrStreamDown.Error() {
		t.Fatalf("got %+v", body)
	}
}

// A channel start never hands the viewer Go or tuner text.
func TestAChannelStartSpeaksToTheViewer(t *testing.T) {
	cases := []struct {
		err     error
		status  int
		code    string
		message string
	}{
		{live.ErrNoSource, http.StatusNotFound, "no_source", live.ErrNoSource.Error()},
		{&live.StreamLimitError{Limit: 2}, http.StatusConflict, "streams_full", "All 2 streams from this playlist are in use. Stop one or raise the limit."},
		{fmt.Errorf("%w (tuner returned 503 Service Unavailable: 805 All Tuners In Use)", live.ErrTunerRefused), http.StatusServiceUnavailable, "tuner_refused", live.ErrTunerRefused.Error()},
		{live.ErrTunerSilent, http.StatusServiceUnavailable, "tuner_silent", "This tuner did not answer. Check that it is on."},
		{sql.ErrNoRows, http.StatusNotFound, "not_found", "That channel is not in the lineup."},
		{errors.New("exec: \"ffmpeg\": executable file not found in $PATH"), http.StatusInternalServerError, "internal", "This channel did not start. The server log says why."},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		watchError(rec, c.err)
		var body struct{ Code, Message string }
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if rec.Code != c.status || body.Code != c.code || body.Message != c.message {
			t.Errorf("%v: got %d %+v", c.err, rec.Code, body)
		}
	}
	rec := httptest.NewRecorder()
	writeError(rec, &live.ExportLimitError{Limit: 32})
	var limited struct {
		Code, Message string
		Limit         int `json:"limit"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &limited); err != nil {
		t.Fatal(err)
	}
	if rec.Code != http.StatusTooManyRequests || limited.Code != "exports_full" || limited.Limit != 32 || !strings.Contains(limited.Message, "32") {
		t.Fatalf("%d %+v", rec.Code, limited)
	}
	rec = httptest.NewRecorder()
	if !refuseFullExport(rec, &live.ExportLimitError{Limit: 32}) || rec.Code != http.StatusTooManyRequests || !strings.Contains(rec.Body.String(), "32") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	// Other routes keep the detail, for settings and diagnostics.
	rec = httptest.NewRecorder()
	writeError(rec, errors.New("dial tcp: connection refused"))
	if !strings.Contains(rec.Body.String(), "connection refused") {
		t.Fatal(rec.Body.String())
	}
}
