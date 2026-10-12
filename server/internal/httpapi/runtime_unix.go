//go:build unix

package httpapi

import (
	"os"
	"runtime"

	"golang.org/x/sys/unix"
)

// openFDs counts this process's file descriptors. The count is for growth,
// not an exact inventory. Darwin's /dev/fd cannot be listed, so there each
// descriptor up to the limit is probed.
func openFDs() int {
	if entries, err := os.ReadDir("/proc/self/fd"); err == nil {
		return len(entries)
	}
	var lim unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &lim); err != nil {
		return -1
	}
	max := lim.Cur
	if max == 0 || max > 65536 {
		max = 65536
	}
	n := 0
	for fd := uintptr(0); fd < uintptr(max); fd++ {
		if _, err := unix.FcntlInt(fd, unix.F_GETFD, 0); err == nil {
			n++
		}
	}
	return n
}

// maxRSS is the resident-set high water, in bytes.
// Linux reports it in kilobytes. Darwin reports bytes.
func maxRSS() uint64 {
	var usage unix.Rusage
	if err := unix.Getrusage(unix.RUSAGE_SELF, &usage); err != nil {
		return 0
	}
	rss := uint64(usage.Maxrss)
	if runtime.GOOS == "linux" {
		rss *= 1024
	}
	return rss
}
