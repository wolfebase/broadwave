package live

import (
	"log"
	"os/exec"
)

// logCommand sends process progress through the process log, which drops
// a password or tuner credential written on the line.
func logCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	cmd.Stderr = log.Writer()
}
