// Package testing provides test doubles for external ironstate handlers.
package testing

import "github.com/TacoContent/ironstate/sdk/handler"

// Context returns an isolated handler context suitable for unit tests.
func Context() handler.Context {
	return handler.Context{Flat: map[string]any{}}
}

// Runner is the command-execution shape handlers commonly depend on.
type Runner interface {
	Run(exe string, args []string) (handler.ExecResult, error)
}

// RecordingRunner records commands and returns the configured result/error.
type RecordingRunner struct {
	Calls  []Command
	Result handler.ExecResult
	Err    error
}

// Command is one command invocation recorded by RecordingRunner.
type Command struct {
	Exe  string
	Args []string
}

// Run records a defensive copy of args and returns the configured outcome.
func (r *RecordingRunner) Run(exe string, args []string) (handler.ExecResult, error) {
	r.Calls = append(r.Calls, Command{Exe: exe, Args: append([]string(nil), args...)})
	return r.Result, r.Err
}
