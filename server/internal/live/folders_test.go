package live

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"broadwave/internal/store"
)

func TestShowNamesFollowPlexAndJellyfin(t *testing.T) {
	at := time.Date(2026, 10, 8, 17, 30, 0, 0, time.Local)
	for _, c := range []struct {
		rec  store.Recording
		want string
	}{
		{store.Recording{Title: "Harbor Watch", Subtitle: "The Lighthouse", Season: 1, Episode: 2}, "TV/Harbor Watch/Season 01/Harbor Watch - S01E02 - The Lighthouse"},
		{store.Recording{Title: "Harbor Watch", Season: 12, Episode: 103}, "TV/Harbor Watch/Season 12/Harbor Watch - S12E103"},
		{store.Recording{Title: "Evening News", Subtitle: "Storm Coverage"}, "TV/Evening News/Evening News - 2026-10-08 - Storm Coverage"},
		{store.Recording{Title: "Evening News"}, "TV/Evening News/Evening News - 2026-10-08 - 1730"},
		{store.Recording{Title: "Night Flight", Category: "Movie", OriginalAir: "1999-04-02"}, "Movies/Night Flight (1999)/Night Flight (1999)"},
		{store.Recording{Title: "Night Flight", Category: "Drama, Movie"}, "Movies/Night Flight/Night Flight"},
		{store.Recording{Title: "Night Flight", Category: "Movie", OriginalAir: "0042"}, "Movies/Night Flight/Night Flight"},
		{store.Recording{Title: "Who? What: When*"}, "TV/Who- What- When-/Who- What- When- - 2026-10-08 - 1730"},
		{store.Recording{Title: "  "}, "TV/KBWV/KBWV - 2026-10-08 - 1730"},
		{store.Recording{Title: "Con", Subtitle: "nul"}, "TV/Con_/Con_ - 2026-10-08 - nul_"},
		{store.Recording{Title: "Night Flight (1999)", Category: "Movie", OriginalAir: "1999"}, "Movies/Night Flight (1999)/Night Flight (1999)"},
		{store.Recording{Title: "Harbor\u202eWatch\ufeff"}, "TV/Harbor Watch/Harbor Watch - 2026-10-08 - 1730"},
	} {
		if got := filepath.ToSlash(showName(c.rec, "KBWV", at)); got != c.want {
			t.Errorf("%+v: %q, want %q", c.rec, got, c.want)
		}
	}
}

// Titles come from the broadcast or a listings feed, so a name must never
// leave its folder, hide, or outgrow what a file system holds.
func TestShowNamesStayInTheirFolder(t *testing.T) {
	at := time.Date(2026, 10, 8, 17, 30, 0, 0, time.Local)
	long := strings.Repeat("é", 200)
	for _, title := range []string{"..", "../../etc/passwd", "/etc/passwd", `..\..\windows`, ".hidden", "...", "a/../../b", "tab\there\x00nul", long, "\xff\xfe"} {
		for _, rec := range []store.Recording{
			{Title: title},
			{Title: title, Subtitle: title, Season: 1, Episode: 1},
			{Title: title, Category: "Movie"},
		} {
			name := showName(rec, "KBWV", at)
			if _, ok := inside("/rec", filepath.Join("/rec", name)); !ok {
				t.Fatalf("%q leaves the folder: %q", title, name)
			}
			for _, part := range strings.Split(name, string(filepath.Separator)) {
				if part == "" || strings.HasPrefix(part, ".") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || len(part) > 200 {
					t.Fatalf("%q makes the part %q in %q", title, part, name)
				}
				if strings.ContainsAny(part, `\:*?"<>|`) || strings.ContainsFunc(part, func(r rune) bool { return r < 0x20 }) {
					t.Fatalf("%q keeps a refused character in %q", title, part)
				}
			}
		}
	}
	if part := folderPart(long); len(part) > 80 || !strings.HasPrefix(long, part) {
		t.Fatalf("long title cut wrong: %d bytes", len(part))
	}
}

func TestRecordingPathPicksTheLayout(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	root := h.Recordings()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	ch := store.SourceChannel{Channel: store.Channel{GuideNumber: "4.1", DisplayName: "KBWV"}}
	rec := store.Recording{Title: "Harbor Watch", Subtitle: "Pilot", Season: 1, Episode: 1, StartedAt: time.Date(2026, 10, 8, 17, 30, 0, 0, time.Local)}
	got := h.recordingPath(rec, ch, true)
	if want := filepath.Join(root, "TV", "Harbor Watch", "Season 01", "Harbor Watch - S01E01 - Pilot.ts"); got != want {
		t.Fatalf("by show: %s, want %s", got, want)
	}
	if info, err := os.Stat(filepath.Dir(got)); err != nil || !info.IsDir() {
		t.Fatalf("season folder not made: %v", err)
	}
	if err := os.WriteFile(got, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if again := h.recordingPath(rec, ch, true); again != strings.TrimSuffix(got, ".ts")+"-2.ts" {
		t.Fatalf("a second copy took %s", again)
	}
	if flat := h.recordingPath(rec, ch, false); flat != filepath.Join(root, "20261008_173000_4.1_KBWV.ts") {
		t.Fatalf("flat: %s", flat)
	}
	// Two games at noon on two channels share a show name; ffmpeg makes the
	// first one's file only after it starts, so the name in use is taken.
	game := store.Recording{Title: "College Football", StartedAt: rec.StartedAt}
	first := h.recordingPath(game, ch, true)
	h.channels = map[int64]*feed{1: {recording: &recording{id: 7, path: first}}}
	if second := h.recordingPath(game, ch, true); second == first || second != strings.TrimSuffix(first, ".ts")+"-2.ts" {
		t.Fatalf("second game got %s (first %s)", second, first)
	}
	h.channels = nil
	// A show folder linked outside the recordings folder is not written through.
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "TV", "Elsewhere")); err != nil {
		t.Fatal(err)
	}
	rec.Title = "Elsewhere"
	if got := h.recordingPath(rec, ch, true); filepath.Dir(got) != root {
		t.Fatalf("followed a link out: %s", got)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("made %v outside", entries)
	}
}

func TestRemovingTheLastEpisodeTakesItsEmptyFolders(t *testing.T) {
	h := &Hub{Dir: t.TempDir()}
	root := h.Recordings()
	season := filepath.Join(root, "TV", "Harbor Watch", "Season 01")
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	first := filepath.Join(season, "Harbor Watch - S01E01.ts")
	second := filepath.Join(season, "Harbor Watch - S01E02.ts")
	for _, path := range []string{first, second, strings.TrimSuffix(first, ".ts") + ".nfo"} {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	h.RemoveRecordingFiles(store.Recording{ID: 1, Path: first})
	if _, err := os.Stat(second); err != nil {
		t.Fatalf("the other episode went: %v", err)
	}
	h.RemoveRecordingFiles(store.Recording{ID: 2, Path: second})
	if _, err := os.Stat(filepath.Join(root, "TV")); !os.IsNotExist(err) {
		t.Fatalf("emptied show folders stayed: %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("the recordings folder went: %v", err)
	}

	// A folder a recording in progress writes into stays, even before its
	// file exists.
	if err := os.MkdirAll(season, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(first, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	h.channels = map[int64]*feed{1: {recording: &recording{id: 3, path: second}}}
	h.RemoveRecordingFiles(store.Recording{ID: 1, Path: first})
	if _, err := os.Stat(season); err != nil {
		t.Fatalf("the folder of a recording in progress went: %v", err)
	}
}
