package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestAddingALibraryFolderAgainRenamesTheShow(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib")
	paths := map[string]string{
		"old":   filepath.Join(lib, "Harbor Watch", "S01", "Harbor Watch - S01E01.mkv"),
		"fresh": filepath.Join(lib, "Harbor Watch", "S01", "Harbor Watch - S01E02.mkv"),
		"kept":  filepath.Join(lib, "Harbor Watch", "S02", "Harbor Watch - S02E01.mkv"),
		"aired": filepath.Join(lib, "Harbor Watch", "S01", "Evening News.ts"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	st := testStore(t)
	ctx := context.Background()
	add := func(rec store.Recording) int64 {
		t.Helper()
		id, err := st.CreateRecording(ctx, rec)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	old := add(store.Recording{
		Title: "S01", GuideNumber: "Library", Path: paths["old"], Status: "complete",
		Category: "Series", Season: 1, Episode: 1, StartedAt: time.Now(),
	})
	if err := st.SaveEpisodePrints(ctx, old, store.EpisodePrints{Head: []uint32{1}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetEpisodeEnds(ctx, old, 40, 70, 0); err != nil {
		t.Fatal(err)
	}
	kept := add(store.Recording{
		Title: "Harbor Watch", GuideNumber: "Library", Path: paths["kept"], Status: "complete",
		Category: "Series", Season: 2, Episode: 1, StartedAt: time.Now(),
	})
	if err := st.SaveEpisodePrints(ctx, kept, store.EpisodePrints{Head: []uint32{2}}); err != nil {
		t.Fatal(err)
	}
	if err := st.SetEpisodeEnds(ctx, kept, 10, 40, 0); err != nil {
		t.Fatal(err)
	}
	aired := add(store.Recording{
		ChannelID: 4, Title: "Evening News", GuideNumber: "4.1", Path: paths["aired"], Status: "complete",
		StartedAt: time.Now(),
	})

	h := (&Server{Store: st}).Handler()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/sources", strings.NewReader(`{"kind":"folder","name":"Shelf","url":"`+lib+`"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}

	got, err := st.Recording(ctx, old)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != "Harbor Watch" || got.Season != 1 || got.Episode != 1 || got.Listened || got.IntroEnd != 0 {
		t.Fatalf("old file: %+v", got)
	}
	keptRec, err := st.Recording(ctx, kept)
	if err != nil {
		t.Fatal(err)
	}
	if keptRec.Title != "Harbor Watch" || !keptRec.Listened || keptRec.IntroEnd != 40 {
		t.Fatalf("a show already named right was heard again: %+v", keptRec)
	}
	if airedRec, _ := st.Recording(ctx, aired); airedRec.Title != "Evening News" {
		t.Fatalf("recording renamed: %+v", airedRec)
	}
	recs, err := st.Recordings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var fresh *store.Recording
	for i := range recs {
		if recs[i].Path == paths["fresh"] {
			fresh = &recs[i]
		}
	}
	if fresh == nil || fresh.Title != "Harbor Watch" || fresh.Season != 1 || fresh.Episode != 2 {
		t.Fatalf("new file: %+v", recs)
	}
}
