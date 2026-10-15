//go:build unix

package remoteexec

import (
	"os/exec"
	"syscall"
)

// isolateSignals puts the child in its own process group so a terminal
// Ctrl-C reaches only the controller, which then cancels gracefully.
func isolateSignals(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
