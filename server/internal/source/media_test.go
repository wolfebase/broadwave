package source

import (
	"os"
	"path/filepath"
	"testing"

	"broadwave/internal/store"
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

func TestScanMediaNamesTheShowAboveASeasonFolder(t *testing.T) {
	root := t.TempDir()
	names := []string{
		filepath.Join(root, "Lighthouse", "S01", "Lighthouse - S01E02.mkv"),
		filepath.Join(root, "Lighthouse", "S02", "Lighthouse - S02E01.mkv"),
		filepath.Join(root, "Lighthouse", "S1", "Lighthouse - S01E03.mkv"),
		filepath.Join(root, "Harbor Watch", "S01", "Harbor Watch - S01E02.mkv"),
		filepath.Join(root, "Lighthouse", "Season.01", "Lighthouse - S01E04.mkv"),
		filepath.Join(root, "Lighthouse", "Season_02", "Lighthouse - S02E02.mkv"),
		filepath.Join(root, "Lighthouse", "Series 1", "Lighthouse - S01E05.mkv"),
		filepath.Join(root, "Lighthouse", "Season 01", "Other Name S01E06.mkv"),
	}
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	type ep struct {
		title string
		se    [2]int
	}
	got := map[ep]int{}
	for _, rec := range found {
		got[ep{rec.Title, [2]int{rec.Season, rec.Episode}}]++
		if rec.Title == "S01" || rec.Title == "S1" || rec.Title == "S02" || rec.Title == "Season.01" || rec.Title == "Season_02" || rec.Title == "Series 1" {
			t.Fatalf("season folder kept as the show: %+v", rec)
		}
	}
	for _, want := range []ep{
		{"Lighthouse", [2]int{1, 2}},
		{"Lighthouse", [2]int{2, 1}},
		{"Lighthouse", [2]int{1, 3}},
		{"Lighthouse", [2]int{1, 4}},
		{"Lighthouse", [2]int{2, 2}},
		{"Lighthouse", [2]int{1, 5}},
		{"Lighthouse", [2]int{1, 6}},
		{"Harbor Watch", [2]int{1, 2}},
	} {
		if got[want] != 1 {
			t.Fatalf("%s %v seen %d times\n%+v", want.title, want.se, got[want], found)
		}
	}
}

func TestLibraryIdentityChangesTheShowAndKeepsARecording(t *testing.T) {
	old := store.Recording{ID: 4, GuideNumber: "Library", Title: "S01", Subtitle: "Harbor Watch - S01E02", Category: "Series", Season: 1, Episode: 2, Listened: true}
	found := store.Recording{Title: "Harbor Watch", Subtitle: "Harbor Watch - S01E02", Category: "Series", Season: 1, Episode: 2, Path: "/lib/a.mkv"}
	next, relisten, ok := LibraryIdentity(old, found)
	if !ok || !relisten || next.ID != 4 || next.Title != "Harbor Watch" {
		t.Fatalf("%+v %v %v", next, relisten, ok)
	}
	found.Title = old.Title
	if _, _, ok := LibraryIdentity(old, found); ok {
		t.Fatal("an unchanged file was renamed")
	}
	old.ChannelID = 3
	old.Title = "S01"
	found.Title = "Harbor Watch"
	if _, _, ok := LibraryIdentity(old, found); ok {
		t.Fatal("a recording was renamed")
	}
	old.ChannelID = 0
	old.Listened = false
	if _, relisten, ok := LibraryIdentity(old, found); !ok || relisten {
		t.Fatal("an unlistened file was marked to be heard again")
	}
}

func TestScanMediaNamesTheShowAboveASeasonSet(t *testing.T) {
	root := t.TempDir()
	names := []string{
		filepath.Join(root, "Lighthouse", "Seasons", "Season 01", "Lighthouse - S01E02.mkv"),
		filepath.Join(root, "Lighthouse", "Season 02", "Lighthouse - S02E03", "episode.mkv"),
		filepath.Join(root, "Harbor Watch", "Seasons", "S01", "Harbor Watch - S01E04", "video.mp4"),
		filepath.Join(root, "Season 03", "Lighthouse - S03E01", "clip.mkv"),
	}
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[[2]int]string{}
	for _, rec := range found {
		got[[2]int{rec.Season, rec.Episode}] = rec.Title
		if rec.Title == "Seasons" || rec.Title == "Season 01" || rec.Title == "Season 02" || rec.Title == "S01" || rec.Title == "Lighthouse - S02E03" || rec.Title == "Harbor Watch - S01E04" {
			t.Fatalf("folder kept as the show: %+v", rec)
		}
	}
	if got[[2]int{1, 2}] != "Lighthouse" || got[[2]int{2, 3}] != "Lighthouse" || got[[2]int{1, 4}] != "Harbor Watch" || got[[2]int{3, 1}] != "Lighthouse" {
		t.Fatalf("%+v", found)
	}
}

func TestScanMediaNamesTheShowAboveSpecials(t *testing.T) {
	root := t.TempDir()
	names := []string{
		filepath.Join(root, "Lighthouse", "Specials", "Lighthouse - S00E03.mkv"),
		filepath.Join(root, "Harbor Watch", "Specials", "Harbor Watch - S00E01.mp4"),
		filepath.Join(root, "Lighthouse", "Seasons", "Specials", "bonus.mkv"),
	}
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]store.Recording{}
	for _, rec := range found {
		got[rec.Path] = rec
	}
	want := []store.Recording{
		{Path: names[0], Title: "Lighthouse", Season: 0, Episode: 3},
		{Path: names[1], Title: "Harbor Watch", Season: 0, Episode: 1},
		{Path: names[2], Title: "Lighthouse", Season: 0, Episode: 0},
	}
	if len(found) != len(want) {
		t.Fatalf("%+v", found)
	}
	for _, w := range want {
		rec := got[w.Path]
		if rec.Title != w.Title || rec.Season != w.Season || rec.Episode != w.Episode || rec.Title == "Specials" {
			t.Fatalf("%+v", rec)
		}
	}
}

// An episode folder under Specials, and one whose show name contains a dot,
// still name the show. The file beside them already does.
func TestScanMediaNamesADottedShowInAnEpisodeFolder(t *testing.T) {
	root := t.TempDir()
	names := []string{
		filepath.Join(root, "Season 1", "Show.Name - S01E02", "episode.mkv"),
		filepath.Join(root, "Season 1", "Show.Name - S01E03.mkv"),
		filepath.Join(root, "Lighthouse", "Specials", "Lighthouse - S00E03", "episode.mkv"),
		filepath.Join(root, "Lighthouse", "Seasons", "Specials", "Lighthouse - S00E04", "episode.mkv"),
	}
	for _, name := range names {
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	found, err := ScanMedia(root)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]store.Recording{}
	for _, rec := range found {
		got[rec.Path] = rec
	}
	want := []store.Recording{
		{Path: names[0], Title: "Show.Name", Season: 1, Episode: 2},
		{Path: names[1], Title: "Show.Name", Season: 1, Episode: 3},
		{Path: names[2], Title: "Lighthouse", Season: 0, Episode: 3},
		{Path: names[3], Title: "Lighthouse", Season: 0, Episode: 4},
	}
	if len(found) != len(want) {
		t.Fatalf("%+v", found)
	}
	for _, w := range want {
		rec := got[w.Path]
		if rec.Title != w.Title || rec.Season != w.Season || rec.Episode != w.Episode {
			t.Fatalf("%+v", rec)
		}
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
