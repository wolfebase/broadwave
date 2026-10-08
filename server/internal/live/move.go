package live

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"broadwave/internal/store"
)

var (
	// ErrMoveName is a name that is empty, absolute, or leaves the recordings folder.
	ErrMoveName = errors.New("use a name inside the recordings folder, such as Show/Episode")
	// ErrMoveTaken is a name another file already has.
	ErrMoveTaken = errors.New("a file with that name is already there")
	// ErrMoveChars is a name with a character some file systems can't hold.
	ErrMoveChars = errors.New(`a name can't hold : * ? " < > |`)
	// ErrMoveOutside is a recording that is not in the recordings folder.
	ErrMoveOutside = errors.New("only recordings in the recordings folder can be moved")
)

// sidecars are the files beside a recording that move and go with it.
var sidecars = []string{".edl", ".json", ".nfo"}

// moveMu keeps two moves from both finding a name free and one replacing
// the other's file.
var moveMu sync.Mutex

// RecordingName is a recording's file relative to the recordings folder,
// with forward slashes; empty for a file outside it.
func (h *Hub) RecordingName(path string) string {
	rel, ok := inside(h.Recordings(), path)
	if !ok {
		return ""
	}
	return filepath.ToSlash(rel)
}

// MoveRecording renames a finished recording and the files beside it to
// name, a path inside the recordings folder; subfolders are made as needed.
// The extension stays the recording's own. It returns the new path.
func (h *Hub) MoveRecording(rec store.Recording, name string) (string, error) {
	moveMu.Lock()
	defer moveMu.Unlock()
	root := h.Recordings()
	if _, ok := inside(root, rec.Path); !ok {
		return "", ErrMoveOutside
	}
	ext := filepath.Ext(rec.Path)
	rel, err := cleanName(name, ext)
	if err != nil {
		return "", err
	}
	target := filepath.Join(root, rel)
	if target == filepath.Clean(rec.Path) {
		return target, nil
	}
	if err := insideReal(root, filepath.Dir(target)); err != nil {
		return "", err
	}
	base, newBase := strings.TrimSuffix(rec.Path, ext), strings.TrimSuffix(target, ext)
	for _, suffix := range append([]string{ext}, sidecars...) {
		info, err := os.Lstat(newBase + suffix)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		// Only the case changed on a disk that ignores case: the name is
		// the file itself, not another one.
		if err == nil && strings.EqualFold(newBase, base) {
			if own, err := os.Lstat(base + suffix); err == nil && os.SameFile(info, own) {
				continue
			}
		}
		return "", ErrMoveTaken
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", err
	}
	// MkdirAll may have followed a link that appeared since the check.
	if err := insideReal(root, filepath.Dir(target)); err != nil {
		return "", err
	}
	if err := os.Rename(rec.Path, target); err != nil {
		return "", fmt.Errorf("could not move the recording: %w", err)
	}
	for _, suffix := range sidecars {
		if err := os.Rename(base+suffix, newBase+suffix); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return target, fmt.Errorf("moved the recording, but not its %s file: %w", suffix, err)
		}
	}
	// Folders the move emptied go; never the recordings folder or a link.
	h.mu.Lock()
	h.pruneEmptyLocked(filepath.Dir(rec.Path))
	h.mu.Unlock()
	return target, nil
}

// cleanName turns a typed name into a relative path with the recording's
// extension, or refuses it.
func cleanName(name, ext string) (string, error) {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if strings.EqualFold(filepath.Ext(name), ext) {
		name = strings.TrimSpace(name[:len(name)-len(ext)])
	}
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsFunc(name, unicode.IsControl) || !utf8.ValidString(name) {
		return "", ErrMoveName
	}
	parts := strings.Split(name, "/")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") || len(part) > 200 {
			return "", ErrMoveName
		}
		if strings.ContainsAny(part, ":*?\"<>|") {
			return "", ErrMoveChars
		}
		parts[i] = part
	}
	return filepath.Join(parts...) + ext, nil
}

func inside(root, path string) (string, bool) {
	if path == "" {
		return "", false
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

// insideReal checks that dir, following links through its deepest folder
// that exists, is still inside root.
func insideReal(root, dir string) error {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	probe := dir
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			return ErrMoveName
		}
		probe = parent
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return err
	}
	if filepath.Clean(real) == filepath.Clean(realRoot) {
		return nil
	}
	if _, ok := inside(realRoot, real); !ok {
		return ErrMoveName
	}
	return nil
}
