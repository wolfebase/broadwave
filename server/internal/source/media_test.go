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

func TestScanMediaNamesTheShowFromAFileInTheLibraryRoot(t *testing.T) {
	root := t.TempDir()
	season := filepath.Join(root, "Season 1")
	show := filepath.Join(root, "Mystery Hour", "Season 2")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(show, 0o755); err != nil {
		t.Fatal(err)
	}
	names := []string{
		filepath.Join(root, "Harbor S01E01.ts"),
		filepath.Join(root, "Harbor - S01E02 - The Storm.mkv"),
		filepath.Join(root, "Harbor_S01E03.ts"),
		filepath.Join(root, "Show.Name.S02E03.Pilot.mp4"),
		filepath.Join(season, "Lighthouse S01E04.ts"),
		filepath.Join(show, "Other Name S02E05.mkv"),
		filepath.Join(root, "S01E01.ts"),
		filepath.Join(root, "S01E02.mp4"),
		filepath.Join(root, "A Western.mp4"),
	}
	for _, name := range names {
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	episodes := map[string]map[[2]int]bool{}
	category := map[string]string{}
	for _, rec := range found {
		if episodes[rec.Title] == nil {
			episodes[rec.Title] = map[[2]int]bool{}
		}
		episodes[rec.Title][[2]int{rec.Season, rec.Episode}] = true
		category[rec.Title] = rec.Category
	}
	harbor := episodes["Harbor"]
	if len(harbor) != 3 || !harbor[[2]int{1, 1}] || !harbor[[2]int{1, 2}] || !harbor[[2]int{1, 3}] {
		t.Fatalf("root files did not share a show: %+v", found)
	}
	if !episodes["Show.Name"][[2]int{2, 3}] {
		t.Fatalf("dotted name: %+v", found)
	}
	if !episodes["Lighthouse"][[2]int{1, 4}] {
		t.Fatalf("season folder at the root: %+v", found)
	}
	if !episodes["Mystery Hour"][[2]int{2, 5}] {
		t.Fatalf("folder name lost: %+v", found)
	}
	if !episodes["S01E01"][[2]int{1, 1}] || !episodes["S01E02"][[2]int{1, 2}] {
		t.Fatalf("a bare episode name was renamed: %+v", found)
	}
	if !episodes["A Western"][[2]int{0, 0}] || category["A Western"] != "Movie" {
		t.Fatalf("a movie got an episode: %+v", found)
	}
	if episodes["Harbor S01E01"] != nil {
		t.Fatalf("filename kept as the show: %+v", found)
	}
}

func TestScanMediaStopsAtTheFileCap(t *testing.T) {
	prev := mediaFileCap
	mediaFileCap = 3
	t.Cleanup(func() { mediaFileCap = prev })

	root := t.TempDir()
	for _, name := range []string{"a.mp4", "b.mp4", "c.mp4", "d.mp4", "e.mp4", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != mediaFileCap {
		t.Fatalf("got %d videos, want %d", len(found), mediaFileCap)
	}
}
