//go:build !windows

package live

import (
	"os/exec"
	"syscall"
)

// killGroupOnCancel starts cmd in its own process group and kills the whole
// group when its context ends.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
