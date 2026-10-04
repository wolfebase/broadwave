package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/live"
	"broadwave/internal/store"
)

func TestARecordingWhoseFileIsGoneSaysSo(t *testing.T) {
	dir := t.TempDir()
	kept := filepath.Join(dir, "recordings", "kept.ts")
	if err := os.MkdirAll(filepath.Dir(kept), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kept, make([]byte, 188), 0o644); err != nil {
		t.Fatal(err)
	}
	st := testStore(t)
	ctx := context.Background()
	add := func(title, status, path string) {
		t.Helper()
		id, err := st.CreateRecording(ctx, store.Recording{Title: title, Status: status, Path: path, StartedAt: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.SetDuration(ctx, id, 2040); err != nil {
			t.Fatal(err)
		}
	}
	add("Kept", "complete", kept)
	add("Gone", "complete", filepath.Join(dir, "recordings", "gone.ts"))
	add("Starting", "recording", filepath.Join(dir, "recordings", "starting.ts"))
	add("Never began", "failed", "")

	h := (&Server{Store: st, Hub: &live.Hub{Store: st, Dir: dir}}).Handler()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/recordings", nil))
	if rr.Code != http.StatusOK {
		t.Fatal(rr.Code, rr.Body.String())
	}
	var body struct {
		Recordings []struct {
			Title    string  `json:"title"`
			Missing  bool    `json:"missing"`
			Bytes    int64   `json:"bytes"`
			Duration float64 `json:"durationSec"`
		} `json:"recordings"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, rec := range body.Recordings {
		seen[rec.Title] = true
		switch rec.Title {
		case "Gone":
			if !rec.Missing || rec.Duration != 0 {
				t.Errorf("gone file: missing %v, duration %v; want true, 0", rec.Missing, rec.Duration)
			}
		case "Kept":
			if rec.Missing || rec.Bytes != 188 || rec.Duration != 2040 {
				t.Errorf("kept file: missing %v, bytes %d, duration %v", rec.Missing, rec.Bytes, rec.Duration)
			}
		default:
			if rec.Missing {
				t.Errorf("%s has no file yet or never had one, and is not missing", rec.Title)
			}
		}
	}
	if len(seen) != 4 {
		t.Fatalf("listed %v", seen)
	}
}
