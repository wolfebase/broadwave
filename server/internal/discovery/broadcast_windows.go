package discovery

import (
	"net"
	"syscall"
)

func enableBroadcast(udp *net.UDPConn) {
	raw, err := udp.SyscallConn()
	if err != nil {
		return
	}
	_ = raw.Control(func(fd uintptr) {
		_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
	})
}
