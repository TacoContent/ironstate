// Package testing provides test doubles and profiling helpers for external ironstate handlers.
package testing

import (
	"fmt"
	"os"
	"runtime/pprof"

	"github.com/TacoContent/ironstate/sdk/handler"
)

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

// Profile captures one profiling run for plugin tests and benchmarks. CPU
// profiles use runtime/pprof's sampling profiler; heap profiles are written
// after fn returns so they describe the completed operation.
func Profile(path string, kind string, fn func() error) error {
	if path == "" {
		return fmt.Errorf("profile path is required")
	}
	if fn == nil {
		return fmt.Errorf("profile function is required")
	}
	if kind != "cpu" && kind != "heap" {
		return fmt.Errorf("unsupported profile kind %q; choose cpu or heap", kind)
	}
	file, err := os.Create(path) //nolint:gosec // caller chooses an explicit profiling output path
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	switch kind {
	case "cpu":
		if err := pprof.StartCPUProfile(file); err != nil {
			return err
		}
		runErr := fn()
		pprof.StopCPUProfile()
		return runErr
	case "heap":
		if err := fn(); err != nil {
			return err
		}
		return pprof.WriteHeapProfile(file)
	}
	return fmt.Errorf("unsupported profile kind %q; choose cpu or heap", kind)
}
