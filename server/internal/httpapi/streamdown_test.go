package httpapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
