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

// seasonDir is a folder that holds one season, not the show. "Season 2",
// "Season.01", "S01", and "Series 1" are seasons. The show is the folder
// above. A season folder at the library root is not a show either.
func seasonDir(name string) bool {
	base := strings.ToLower(strings.TrimSpace(name))
	rest, ok := strings.CutPrefix(base, "season")
	if !ok {
		rest, ok = strings.CutPrefix(base, "series")
	}
	if ok {
		rest = strings.TrimLeft(rest, " ._-")
		n, err := strconv.Atoi(rest)
		return err == nil && rest != "" && n >= 0 && n < 100
	}
	if len(base) < 2 || len(base) > 3 || base[0] != 's' {
		return false
	}
	n, err := strconv.Atoi(base[1:])
	return err == nil && n >= 0 && n < 100
}

// specialsDir is a folder of one-offs, not the show. The show is above it.
func specialsDir(name string) bool {
	return strings.EqualFold(strings.TrimSpace(name), "specials")
}

// showBeforeEpisode is the show in a filename that has no folder to name it.
// "Harbor - S01E02 - The Storm" is Harbor. A name that is only the episode
// is left alone, so two bare files do not become one show.
func showBeforeEpisode(name string) (string, bool) {
	base := name
	// Only a video extension comes off. A dot in the show name is not one,
	// and an episode folder has no extension at all.
	switch strings.ToLower(filepath.Ext(name)) {
	case ".ts", ".mp4", ".mkv", ".m4v", ".mov":
		base = strings.TrimSuffix(name, filepath.Ext(name))
	}
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
		// Show/Season 01/Show - S01E02/episode, or Show/Specials/Show - S00E03/episode.
		episodeDir := ""
		if dir != root {
			parent := filepath.Dir(dir)
			if (seasonDir(filepath.Base(parent)) || specialsDir(filepath.Base(parent))) && episodeFile.MatchString(filepath.Base(dir)) {
				episodeDir = filepath.Base(dir)
				dir = parent
			}
		}
		climbed := false
		if (seasonDir(filepath.Base(dir)) || specialsDir(filepath.Base(dir))) && dir != root {
			// Show/S01/episode, or Show/Specials/episode: the show is above.
			dir = filepath.Dir(dir)
			climbed = true
		}
		// Show/Seasons/Season 01/episode: the set folder is not the show either.
		if climbed && strings.EqualFold(strings.TrimSpace(filepath.Base(dir)), "seasons") && dir != root {
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
		label := info.Name()
		if episodeFile.FindStringSubmatch(label) == nil && episodeDir != "" {
			label = episodeDir
		}
		if m := episodeFile.FindStringSubmatch(label); m != nil {
			category = "Series"
			if subtitle == "" {
				subtitle = "S" + m[1] + "E" + m[2]
			}
			season, _ = strconv.Atoi(m[1])
			episode, _ = strconv.Atoi(m[2])
			// A file dropped in the library root has no show folder. The
			// name before SxxEyy is the show, so two episodes are one series.
			if !namedByFolder {
				if show, ok := showBeforeEpisode(label); ok {
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

// LibraryIdentity is how a file already in the library should be named, when
// the scan now reads a different show, season, or episode. ok is false when
// the row is not a library file, or the name is already right. relisten is
// set when that change would group it with different episodes, so an intro
// found under the old name is dropped.
func LibraryIdentity(old, found store.Recording) (next store.Recording, relisten, ok bool) {
	if old.ChannelID != 0 || old.GuideNumber != "Library" {
		return store.Recording{}, false, false
	}
	if old.Title == found.Title && old.Subtitle == found.Subtitle && old.Category == found.Category && old.Season == found.Season && old.Episode == found.Episode {
		return store.Recording{}, false, false
	}
	relisten = old.Listened && (old.Title != found.Title || old.Season != found.Season || old.Episode != found.Episode)
	next = found
	next.ID = old.ID
	return next, relisten, true
}
