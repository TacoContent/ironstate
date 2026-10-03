//go:build windows

package remoteexec

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// isolateSignals puts the child in a new process group, which does not
// receive the console's Ctrl-C, so only the controller handles it.
func isolateSignals(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}
