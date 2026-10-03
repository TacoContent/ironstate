// Package remoteexec is the controller side of remote apply
// (docs/plans/remote-apply.md): it packages a playbook into a job, starts
// an 'ironstate agent' over a Transport, and collects its event stream.
package remoteexec

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// RemoteCommand is a structured command for a Transport; never a shell
// string built from user input.
type RemoteCommand struct {
	Program string
	Args    []string
}

// Transport runs commands on one target.
type Transport interface {
	// Exec runs cmd, streaming stdin/stdout/stderr, and returns its exit
	// code. err is reserved for failures to run it at all.
	Exec(ctx context.Context, cmd RemoteCommand, stdin io.Reader, stdout, stderr io.Writer) (int, error)
	Close() error
}

// LocalTransport runs the agent as a local child process (no SSH). Used by
// tests and by the reserved 'local' target.
type LocalTransport struct {
	// Env is appended to the current process environment.
	Env []string
}

// Exec implements Transport.
func (t LocalTransport) Exec(ctx context.Context, cmd RemoteCommand, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	c := exec.CommandContext(ctx, cmd.Program, cmd.Args...) //nolint:gosec // program is the ironstate binary chosen by the controller
	c.Env = append(os.Environ(), t.Env...)
	c.Stdin = stdin
	c.Stdout = stdout
	c.Stderr = stderr
	c.WaitDelay = waitDelay
	isolateSignals(c)
	return runCommand(c)
}

// waitDelay bounds how long Exec waits for I/O after the process exits.
const waitDelay = 5 * time.Second

func runCommand(c *exec.Cmd) (int, error) {
	err := c.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), nil
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return c.ProcessState.ExitCode(), nil
	}
	if err != nil {
		return -1, err
	}
	return 0, nil
}

// Close implements Transport.
func (LocalTransport) Close() error { return nil }
