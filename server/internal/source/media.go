package source

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"waveguide/internal/store"
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
		if parent := filepath.Base(filepath.Dir(path)); parent != filepath.Base(root) && !strings.EqualFold(parent, "Season 1") && !strings.HasPrefix(strings.ToLower(parent), "season ") {
			title = parent
			subtitle = strings.TrimSuffix(info.Name(), filepath.Ext(info.Name()))
			category = "Series"
		}
		if m := episodeFile.FindStringSubmatch(info.Name()); m != nil {
			category = "Series"
			if subtitle == "" {
				subtitle = "S" + m[1] + "E" + m[2]
			}
		}
		out = append(out, store.Recording{
			Title: title, Subtitle: subtitle, Category: category, Path: path, Status: "complete", GuideNumber: "Library",
		})
		return nil
	})
	return out, err
}
