//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// protectStdout moves the real stdout handle to a private duplicate for
// protocol events and makes the process's standard output handle stderr.
func protectStdout() (*os.File, error) {
	proc := windows.CurrentProcess()
	out, err := windows.GetStdHandle(windows.STD_OUTPUT_HANDLE)
	if err != nil {
		return nil, err
	}
	var dup windows.Handle
	if err := windows.DuplicateHandle(proc, out, proc, &dup, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, err
	}
	errHandle, err := windows.GetStdHandle(windows.STD_ERROR_HANDLE)
	if err != nil {
		_ = windows.CloseHandle(dup)
		return nil, err
	}
	if err := windows.SetStdHandle(windows.STD_OUTPUT_HANDLE, errHandle); err != nil {
		_ = windows.CloseHandle(dup)
		return nil, err
	}
	return os.NewFile(uintptr(dup), "protocol"), nil
}

// agentSignals is a no-op on Windows (no SIGHUP/SIGPIPE).
func agentSignals() func() { return func() {} }
