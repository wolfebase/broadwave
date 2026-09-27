package disk

import (
	"errors"
	"os"
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
	return "The recordings disk is full. Free some space, then try again."
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
	return "Broadwave can't save this recording. Check the recordings folder, then try again."
}

// Writable creates and removes a probe file. A recording is refused when this fails,
// before a tuner is taken.
func Writable(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return writeFailure(err)
	}
	f, err := os.CreateTemp(dir, ".bw-*")
	if err != nil {
		return writeFailure(err)
	}
	name := f.Name()
	_ = f.Close()
	_ = os.Remove(name)
	return nil
}

func writeFailure(err error) error {
	if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EROFS) {
		return &WriteError{Full: true}
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
