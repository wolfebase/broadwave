//go:build windows

package disk

import (
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func Stat(path string) (Space, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return Space{}, err
	}
	var avail, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(name, &avail, &total, &free); err != nil {
		return Space{}, err
	}
	return Space{Free: avail, Total: total}, nil
}

// SameDevice reports whether two paths are on one volume.
func SameDevice(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	return errA == nil && errB == nil && strings.EqualFold(filepath.VolumeName(a), filepath.VolumeName(b))
}
