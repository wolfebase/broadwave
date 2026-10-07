package dvr

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"broadwave/internal/breaks"
	"broadwave/internal/store"
)

func scanFinds(t *testing.T, found ...breaks.Break) {
	t.Helper()
	old := indexFile
	indexFile = func(string, string) ([]breaks.Break, error) { return found, nil }
	t.Cleanup(func() { indexFile = old })
}

// The scan at the end of a recording keeps breaks marked by hand while it
// recorded; Find commercials starts over.
func TestTheEndOfRecordingScanKeepsHandMarkers(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	path := filepath.Join(t.TempDir(), "news.ts")
	if err := os.WriteFile(path, []byte{0x47}, 0o644); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", Path: path, Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := st.Recording(ctx, id)
	if _, err := st.AddMarker(ctx, id, 100, 200); err != nil {
		t.Fatal(err)
	}
	scanFinds(t, breaks.Break{Start: 98, End: 210, Confidence: 0.95}, breaks.Break{Start: 600, End: 700, Confidence: 0.6})
	got, err := IndexBreaks(ctx, st, "", rec, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Start != 100 || got[0].Confidence != 1 || got[1].Start != 600 || got[1].Confidence != 0.6 {
		t.Fatalf("%+v", got)
	}
	got, err = IndexBreaks(ctx, st, "", rec, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Start != 98 || got[0].Confidence != 0.95 {
		t.Fatalf("%+v", got)
	}
}
