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
	indexFile = func(string, string, []breaks.Spot) (breaks.Result, error) { return breaks.Result{Breaks: found}, nil }
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

// A scan is handed the spot library, and keeps what it learned: new spots
// stored, known ones marked seen.
func TestIndexBreaksLearnsSpots(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.AddSpots(ctx, 99, [][]uint64{{11, 12}, {21, 22}}); err != nil {
		t.Fatal(err)
	}
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", Path: "/data/recordings/news.ts", Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := st.Recording(ctx, id)
	var handed []breaks.Spot
	old := indexFile
	indexFile = func(_, _ string, known []breaks.Spot) (breaks.Result, error) {
		handed = known
		return breaks.Result{Seen: []bool{false, true}, Spots: []breaks.Spot{{Prints: []uint64{31, 32, 33}, Start: 100}}}, nil
	}
	t.Cleanup(func() { indexFile = old })
	if _, err := IndexBreaks(ctx, st, "", rec, true); err != nil {
		t.Fatal(err)
	}
	if len(handed) != 2 || handed[1].Prints[0] != 21 || handed[1].Start != -1 {
		t.Fatalf("handed %+v", handed)
	}
	spots, err := st.Spots(ctx)
	if err != nil || len(spots) != 3 || spots[2].Prints[2] != 33 {
		t.Fatalf("%+v %v", spots, err)
	}
	if rec, _ := st.Recording(ctx, id); !rec.BreaksScanned {
		t.Fatal("not marked scanned")
	}
}

func TestIndexBreaksDoesNotMarkAFileStillRecording(t *testing.T) {
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
	id, err := st.CreateRecording(ctx, store.Recording{
		Title: "News", Path: path, Status: "recording", StartedAt: time.Now(),
	})
	if err != nil {
		t.Fatal(err)
	}
	rec, err := st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	scanFinds(t, breaks.Break{Start: 10, End: 40, Confidence: 0.9})
	if _, err := IndexBreaks(ctx, st, "", rec, false); err != nil {
		t.Fatal(err)
	}
	rec, err = st.Recording(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if rec.BreaksScanned {
		t.Fatal("detect while recording set breaks_scanned")
	}
}

// Scanning a recording again does not match it against its own spots.
func TestIndexBreaksSkipsItsOwnSpots(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "cfg"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	id, err := st.CreateRecording(ctx, store.Recording{Title: "News", Path: "/data/recordings/news.ts", Status: "complete", StartedAt: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddSpots(ctx, id, [][]uint64{{1, 2}}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddSpots(ctx, id+1, [][]uint64{{3, 4}}); err != nil {
		t.Fatal(err)
	}
	rec, _ := st.Recording(ctx, id)
	var handed []breaks.Spot
	old := indexFile
	indexFile = func(_, _ string, known []breaks.Spot) (breaks.Result, error) {
		handed = known
		return breaks.Result{}, nil
	}
	t.Cleanup(func() { indexFile = old })
	if _, err := IndexBreaks(ctx, st, "", rec, false); err != nil {
		t.Fatal(err)
	}
	if len(handed) != 1 || handed[0].Prints[0] != 3 {
		t.Fatalf("handed %+v", handed)
	}
}
