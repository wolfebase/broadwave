package live

import "os/exec"

// killGroupOnCancel leaves Windows to exec's own kill of the direct child.
func killGroupOnCancel(cmd *exec.Cmd) {}
