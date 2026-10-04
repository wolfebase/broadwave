//go:build !unix

package discovery

import "syscall"

func sharePort(_, _ string, _ syscall.RawConn) error { return nil }
