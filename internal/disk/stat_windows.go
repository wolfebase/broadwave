//go:build windows

package disk

import "golang.org/x/sys/windows"

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
