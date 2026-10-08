package live

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"broadwave/internal/nfo"
	"broadwave/internal/store"
)

// FoldersByShow reports whether new recordings go in the layout Plex and
// Jellyfin read (the folderLayout setting; anything but "flat").
func FoldersByShow(values map[string]string) bool {
	return values["folderLayout"] != "flat"
}

// showName is a recording's file name inside the recordings folder, without
// the extension, in the layout Plex and Jellyfin read:
//
//	TV/Show/Season 01/Show - S01E02 - Episode
//	TV/Show/Show - 2026-10-08 - Episode (or the start time when unnamed)
//	Movies/Title (1999)/Title (1999)
//
// Titles come from the guide, so every part is cleaned to one folder name.
func showName(rec store.Recording, channel string, started time.Time) string {
	title := folderPart(rec.Title)
	if title == "" {
		title = folderPart(channel)
	}
	if title == "" {
		title = "Recording"
	}
	if nfo.IsMovie(rec.Category) {
		if year := airYear(rec.OriginalAir); year != "" && !strings.HasSuffix(title, "("+year+")") {
			title += " (" + year + ")"
		}
		return filepath.Join("Movies", title, title)
	}
	subtitle := folderPart(rec.Subtitle)
	if rec.Season > 0 && rec.Episode > 0 {
		name := fmt.Sprintf("%s - S%02dE%02d", title, rec.Season, rec.Episode)
		if subtitle != "" {
			name += " - " + subtitle
		}
		return filepath.Join("TV", title, fmt.Sprintf("Season %02d", rec.Season), name)
	}
	if subtitle == "" {
		subtitle = started.Format("1504")
	}
	return filepath.Join("TV", title, title+" - "+started.Format("2006-01-02")+" - "+subtitle)
}

// folderPart makes one file or folder name from guide text: no separators,
// no characters Windows shares refuse, no leading dots (a hidden file, or
// ".."), no trailing dots or spaces, and at most 80 bytes, so a whole name
// with a title, an episode code, and a subtitle stays far under 255.
func folderPart(text string) string {
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	text = strings.Map(func(r rune) rune {
		switch {
		case strings.ContainsRune(`/\:*?"<>|`, r):
			return '-'
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			return ' '
		}
		return r
	}, text)
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 80 {
		cut := 80
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut]
	}
	text = strings.TrimRight(strings.TrimLeft(text, ". "), ". ")
	// Windows refuses these as names, so a share of the folder would too.
	stem, _, _ := strings.Cut(strings.ToUpper(text), ".")
	switch strings.TrimSpace(stem) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		text += "_"
	}
	return text
}

func airYear(air string) string {
	if len(air) < 4 {
		return ""
	}
	year, err := strconv.Atoi(air[:4])
	if err != nil || year < 1880 || year > time.Now().Year()+1 {
		return ""
	}
	return air[:4]
}

// pruneEmpty removes dir and the folders above it while they are empty,
// stopping at the recordings folder. Links are left alone, and so is a
// folder a recording in progress writes into: its file may not exist yet.
// moveMu keeps it from taking a folder a move has just made.
func (h *Hub) pruneEmpty(dir string) {
	moveMu.Lock()
	defer moveMu.Unlock()
	h.mu.Lock()
	defer h.mu.Unlock()
	h.pruneEmptyLocked(dir)
}

func (h *Hub) pruneEmptyLocked(dir string) {
	root := h.Recordings()
	var busy []string
	for _, f := range h.feedsLocked() {
		if f.recording != nil && !f.recording.finished {
			busy = append(busy, f.recording.path)
		}
	}
	for {
		if _, ok := inside(root, dir); !ok || insideReal(root, dir) != nil {
			return
		}
		for _, path := range busy {
			if _, ok := inside(dir, path); ok {
				return
			}
		}
		info, err := os.Lstat(dir)
		if err != nil || !info.IsDir() || os.Remove(dir) != nil {
			return
		}
		dir = filepath.Dir(dir)
	}
}
