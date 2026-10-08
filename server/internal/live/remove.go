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
// poster and file-playback work. A file in a library folder is left alone.
func (h *Hub) RemoveRecordingFiles(rec store.Recording) {
	root := h.Recordings()
	workDir := h.Dir
	removeInside(root, rec.Path)
	base := strings.TrimSuffix(rec.Path, filepath.Ext(rec.Path))
	removeInside(root, base+".edl")
	removeInside(root, base+".json")
	removeInside(root, base+".nfo")
	_ = os.Remove(filepath.Join(workDir, "posters", strconv.FormatInt(rec.ID, 10)+".jpg"))
	_ = os.RemoveAll(filepath.Join(workDir, "file", strconv.FormatInt(rec.ID, 10)))
}

func removeInside(root, path string) {
	if path == "" {
		return
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(root), clean)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	_ = os.Remove(clean)
}
