//go:build unix

package discovery

import (
	"syscall"

	"golang.org/x/sys/unix"
)

// sharePort lets every Broadwave server on a computer bind FinderPort. Each
// gets its own copy of a broadcast probe; a probe sent to one address reaches
// only one of them.
func sharePort(_, _ string, c syscall.RawConn) error {
	var err error
	if cerr := c.Control(func(fd uintptr) {
		if err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
			return
		}
		err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEPORT, 1)
	}); cerr != nil {
		return cerr
	}
	return err
}
