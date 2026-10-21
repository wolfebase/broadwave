package disk

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// Space is the filesystem that holds recordings.
type Space struct {
	Free  uint64
	Total uint64
}

// LowError means a new recording would cross the free-space reserve.
type LowError struct {
	Free uint64
	Need uint64
}

func (e *LowError) Error() string {
	return fmt.Sprintf("The recordings disk has %s free, and Broadwave keeps %s in reserve. Free some space or lower the reserve in Settings.", gigabytes(e.Free), gigabytes(e.Need))
}

func gigabytes(n uint64) string {
	whole := n / 1_000_000_000
	tenth := (n % 1_000_000_000) / 100_000_000
	if tenth == 0 {
		return fmt.Sprintf("%d GB", whole)
	}
	return fmt.Sprintf("%d.%d GB", whole, tenth)
}

// WriteError means the recordings folder cannot take a new file.
// Full is a disk with no room left. Otherwise the folder refused the write.
type WriteError struct {
	Full bool
}

func (e *WriteError) Error() string {
	if e.Full {
		return "The recordings disk is full. Free some space, then try again."
	}
	return "Broadwave can't write to the recordings folder. Check PUID/PGID or the mount."
}

// MissingError means the recordings folder this server has used is not there,
// or is there without the mark the server left in it: a drive or share that
// is not mounted. The server does not make a new folder in its place, where
// recordings would fill the disk under the mount point.
type MissingError struct {
	Dir string
}

func (e *MissingError) Error() string {
	return "The recordings folder isn't there. Is the drive mounted?"
}

// Marker is the file the server leaves in a recordings folder it uses.
const Marker = ".broadwave-recordings"

// seenFile, in the server's own folder, names the recordings folder that had the mark.
const seenFile = "recordings-folder"

// Prepare makes dir ready for a recording. A folder the server has used
// before (noted in stateDir) must still be there with its mark. known says
// the catalog holds recordings in dir: an older server left no note, so a
// missing or empty folder then counts as used before too. A new folder is
// made and marked.
func Prepare(dir, stateDir string, known bool) error {
	dir = filepath.Clean(dir)
	seen := false
	if stateDir != "" {
		if b, err := os.ReadFile(filepath.Join(stateDir, seenFile)); err == nil && filepath.Clean(strings.TrimSpace(string(b))) == dir {
			seen = true
		}
	}
	marker := filepath.Join(dir, Marker)
	info, err := os.Stat(dir)
	switch {
	case err == nil && !info.IsDir():
		return &WriteError{}
	case err == nil:
		if _, err := os.Stat(marker); err == nil {
			return note(stateDir, dir, seen)
		}
		if seen || (known && emptyDir(dir)) {
			return &MissingError{Dir: dir}
		}
	case errors.Is(err, fs.ErrNotExist):
		if seen || known {
			return &MissingError{Dir: dir}
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return writeFailure(err)
		}
	default:
		return writeFailure(err)
	}
	if err := os.WriteFile(marker, []byte("Broadwave saves recordings in this folder. Leave this file here: without it the server takes the folder for a drive that is not mounted.\n"), 0o644); err != nil {
		return writeFailure(err)
	}
	return note(stateDir, dir, seen)
}

func note(stateDir, dir string, seen bool) error {
	if seen || stateDir == "" {
		return nil
	}
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		return nil
	}
	_ = os.WriteFile(filepath.Join(stateDir, seenFile), []byte(dir+"\n"), 0o644)
	return nil
}

func emptyDir(dir string) bool {
	f, err := os.Open(dir)
	if err != nil {
		return false
	}
	defer f.Close()
	names, _ := f.Readdirnames(1)
	return len(names) == 0
}

// Writable creates and removes a probe file. A recording is refused when this fails,
// before a tuner is taken. It does not make the folder; Prepare does.
func Writable(dir string) error {
	f, err := os.CreateTemp(dir, ".bw-*")
	if err != nil {
		return writeFailure(err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

// ClassifyWrite turns a recordings-folder failure into a WriteError.
// A full disk is ENOSPC. A read-only folder stays the folder message.
// Errors that already name the disk are left as they are.
func ClassifyWrite(err error) error {
	if err == nil {
		return nil
	}
	var low *LowError
	var blocked *WriteError
	var missing *MissingError
	if errors.As(err, &low) || errors.As(err, &blocked) || errors.As(err, &missing) {
		return err
	}
	if errors.Is(err, syscall.ENOSPC) {
		return &WriteError{Full: true}
	}
	if os.IsPermission(err) || errors.Is(err, syscall.EROFS) {
		return &WriteError{}
	}
	return err
}

func writeFailure(err error) error {
	if classified := ClassifyWrite(err); classified != err {
		return classified
	}
	return &WriteError{}
}

// WatermarkGB is the reserve in gigabytes. An empty value uses 10. Zero turns the reserve off.
func WatermarkGB(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 10
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 10
	}
	if n > 1000000 {
		return 1000000
	}
	return n
}

func WatermarkBytes(raw string) uint64 {
	gb := WatermarkGB(raw)
	if gb == 0 {
		return 0
	}
	return uint64(gb) * 1000 * 1000 * 1000
}

func BelowReserve(free, reserve uint64) bool {
	return reserve > 0 && free < reserve
}
