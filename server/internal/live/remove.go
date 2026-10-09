package live

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"broadwave/internal/store"
)

// RemoveRecordingFiles deletes a recording's file and the files beside it
// (.edl, .json, .nfo) when they sit inside the recordings folder, plus its
// poster and file-playback work, and the show and season folders it empties.
// A file in a library folder is left alone.
func (h *Hub) RemoveRecordingFiles(rec store.Recording) {
	root := h.Recordings()
	workDir := h.Dir
	removeInside(root, rec.Path)
	base := strings.TrimSuffix(rec.Path, filepath.Ext(rec.Path))
	removeInside(root, base+".edl")
	removeInside(root, base+".json")
	removeInside(root, base+".nfo")
	h.pruneEmpty(filepath.Dir(rec.Path))
	_ = os.Remove(filepath.Join(workDir, "posters", strconv.FormatInt(rec.ID, 10)+".jpg"))
	_ = os.RemoveAll(filepath.Join(workDir, "file", strconv.FormatInt(rec.ID, 10)))
}

func removeInside(root, path string) {
	if path == "" {
		return
	}
	clean := filepath.Clean(path)
	if _, ok := inside(root, clean); !ok {
		return
	}
	// Lstat does not follow the final name. A link inside the folder is
	// removed. A parent link that leaves the folder is not.
	if _, err := os.Lstat(clean); err != nil {
		return
	}
	if err := insideReal(root, filepath.Dir(clean)); err != nil {
		return
	}
	_ = os.Remove(clean)
}
