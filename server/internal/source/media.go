package source

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"broadwave/internal/store"
)

var episodeFile = regexp.MustCompile(`(?i)[sS](\d{1,2})[eE](\d{1,3})`)

// showBeforeEpisode is the show in a filename that has no folder to name it.
// "Harbor - S01E02 - The Storm" is Harbor. A name that is only the episode
// is left alone, so two bare files do not become one show.
func showBeforeEpisode(name string) (string, bool) {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	loc := episodeFile.FindStringIndex(base)
	if loc == nil || loc[0] == 0 {
		return "", false
	}
	title := strings.TrimRight(base[:loc[0]], " .-_")
	if title == "" {
		return "", false
	}
	return title, true
}

// mediaFileCap is the most videos ScanMedia returns from one folder.
var mediaFileCap = 200_000

// ScanMedia reads a folder of movies and episodes. Files stay where they are.
func ScanMedia(root string) ([]store.Recording, error) {
	root = filepath.Clean(root)
	var out []store.Recording
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ts", ".mp4", ".mkv", ".m4v", ".mov":
		default:
			return nil
		}
		title := strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
		subtitle := ""
		category := "Movie"
		dir := filepath.Dir(path)
		if strings.HasPrefix(strings.ToLower(filepath.Base(dir)), "season ") && dir != root {
			// Show/Season 2/episode: the show is the folder above.
			dir = filepath.Dir(dir)
		}
		namedByFolder := false
		if dir != root && strings.HasPrefix(dir, root) {
			title = filepath.Base(dir)
			subtitle = strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
			category = "Series"
			namedByFolder = true
		}
		season, episode := 0, 0
		if m := episodeFile.FindStringSubmatch(info.Name()); m != nil {
			category = "Series"
			if subtitle == "" {
				subtitle = "S" + m[1] + "E" + m[2]
			}
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
			// A file dropped in the library root has no show folder. The
			// name before SxxEyy is the show, so two episodes are one series.
			if !namedByFolder {
				if show, ok := showBeforeEpisode(info.Name()); ok {
					title = show
				}
			}
		}
		out = append(out, store.Recording{
			Title: title, Subtitle: subtitle, Category: category, Path: path, Status: "complete", GuideNumber: "Library",
			Season: season, Episode: episode,
		})
		if len(out) >= mediaFileCap {
			return filepath.SkipAll
		}
		return nil
	})
	return out, err
}
