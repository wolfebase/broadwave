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
// A file in a library folder is left alone. The error is from the recording
// file itself. Sidecars are best-effort, and a file that is already gone is
// not an error.
func (h *Hub) RemoveRecordingFiles(rec store.Recording) error {
	root := h.Recordings()
	workDir := h.Dir
	err := removeInside(root, rec.Path)
	base := strings.TrimSuffix(rec.Path, filepath.Ext(rec.Path))
	_ = removeInside(root, base+".edl")
	_ = removeInside(root, base+".json")
	_ = removeInside(root, base+".nfo")
	h.pruneEmpty(filepath.Dir(rec.Path))
	_ = os.Remove(filepath.Join(workDir, "posters", strconv.FormatInt(rec.ID, 10)+".jpg"))
	_ = os.RemoveAll(filepath.Join(workDir, "file", strconv.FormatInt(rec.ID, 10)))
	return err
}

func removeInside(root, path string) error {
	if path == "" {
		return nil
	}
	clean := filepath.Clean(path)
	rel, err := filepath.Rel(filepath.Clean(root), clean)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil
	}
	err = os.Remove(clean)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
