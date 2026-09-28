package nfo

import (
	"os"
	"path/filepath"
	"strings"

	"broadwave/internal/store"
)

// Inside reports whether candidate resolves inside root. A symlink that
// leaves root does not. A missing file is judged by its cleaned path.
// The same rule guards a recording download.
func Inside(root, candidate string) bool {
	root = filepath.Clean(root)
	if root == "" || root == "." {
		return false
	}
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	candidate = filepath.Clean(candidate)
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
		candidate = resolved
	} else if !os.IsNotExist(err) {
		return false
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	return true
}

// Side is the .nfo path beside a media file.
func Side(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".nfo"
}

// Write saves the .nfo beside a finished recording whose file is inside root.
// A recording still in progress, a missing file, and any path outside root
// (a library folder, a "..", a symlink) are left alone.
func Write(root string, rec store.Recording) (bool, error) {
	if rec.Status != "complete" || rec.Path == "" || root == "" {
		return false, nil
	}
	clean := filepath.Clean(rec.Path)
	// The media file and the folder the sidecar lands in must both resolve
	// inside root: a link in a library folder can point at a recording. An
	// existing sidecar that is a link is not followed.
	if !Inside(root, clean) || !Inside(root, filepath.Dir(clean)) {
		return false, nil
	}
	info, err := os.Stat(clean)
	if err != nil || !info.Mode().IsRegular() {
		return false, nil
	}
	side := Side(clean)
	if info, err := os.Lstat(side); err == nil && !info.Mode().IsRegular() {
		return false, nil
	}
	body := Document(rec)
	if len(body) == 0 {
		return false, os.ErrInvalid
	}
	if err := os.WriteFile(side, body, 0o644); err != nil {
		return false, err
	}
	return true, nil
}

// Refresh writes an .nfo for each finished recording inside root.
func Refresh(root string, recs []store.Recording) (int, error) {
	n := 0
	var first error
	for _, rec := range recs {
		ok, err := Write(root, rec)
		if err != nil && first == nil {
			first = err
		}
		if ok {
			n++
		}
	}
	return n, first
}
