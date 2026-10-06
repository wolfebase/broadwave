package source

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanMediaReadsSeasonAndEpisode(t *testing.T) {
	root := t.TempDir()
	show := filepath.Join(root, "Mystery Hour", "Season 2")
	if err := os.MkdirAll(show, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{filepath.Join(show, "Mystery Hour S02E05.mkv"), filepath.Join(root, "A Western.mp4")} {
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][2]int{}
	for _, rec := range found {
		got[rec.Title] = [2]int{rec.Season, rec.Episode}
	}
	if got["Mystery Hour"] != [2]int{2, 5} {
		t.Fatalf("episode not read: %+v", found)
	}
	if got["A Western"] != [2]int{0, 0} {
		t.Fatalf("a movie got an episode: %+v", found)
	}
}
