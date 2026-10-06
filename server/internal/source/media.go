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
		if dir != root && strings.HasPrefix(dir, root) {
			title = filepath.Base(dir)
			subtitle = strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
			category = "Series"
		}
		season, episode := 0, 0
		if m := episodeFile.FindStringSubmatch(info.Name()); m != nil {
			category = "Series"
			if subtitle == "" {
				subtitle = "S" + m[1] + "E" + m[2]
			}
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
		}
		out = append(out, store.Recording{
			Title: title, Subtitle: subtitle, Category: category, Path: path, Status: "complete", GuideNumber: "Library",
			Season: season, Episode: episode,
		})
		return nil
	})
	return out, err
}
