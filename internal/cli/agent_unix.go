//go:build unix

package cli

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/sys/unix"
)

// protectStdout moves the real stdout to a private descriptor for protocol
// events and points fd 1 at stderr, so nothing else can write into the
// protocol stream - not even code that captured os.Stdout earlier.
func protectStdout() (*os.File, error) {
	fd, err := unix.Dup(1)
	if err != nil {
		return nil, err
	}
	unix.CloseOnExec(fd)
	if err := dupOnto(2, 1); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), "protocol"), nil
}

// agentSignals keeps a dropped SSH session from killing the agent: SIGHUP
// is ignored and SIGPIPE is delivered (and ignored) instead of killing the
// process on a write to a closed stdout/stderr.
func agentSignals() func() {
	signal.Ignore(syscall.SIGHUP)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGPIPE)
	return func() { signal.Stop(ch) }
}
