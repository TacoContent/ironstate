package exec

// Package exec wraps external command invocation behind a small
// interface (docs/plans/go-rewrite.md §4.8) so every CLI-backed handler
// (winget, chocolatey, pipx, npm, cargo, go, gem, eget) is unit-testable
// with a fake Runner asserting the exact argv built, without touching a
// real package manager.

import (
	"bytes"
	"os/exec"
	"strings"
)

// Result is a completed command's captured output — the same shape every
// handler normalizes its own result to (docs/plans/go-rewrite.md §2).
type Result struct {
	RC          int
	Stdout      string
	StdoutLines []string
	Stderr      string
	StderrLines []string
}

// Runner runs an external command, capturing stdout/stderr separately.
type Runner interface {
	Run(exe string, args []string) (Result, error)
}

// Default is the real, subprocess-backed Runner. Overridden in tests via
// a package-level var, matching this codebase's existing pattern (see
// internal/handlers/fact.go's runPwshCommand, internal/filters/lookup.go).
var Default Runner = realRunner{}

type realRunner struct{}

func (realRunner) Run(exe string, args []string) (Result, error) {
	cmd := exec.Command(exe, args...) //nolint:gosec // exe/args come from authored YAML content, same trust boundary as every other CLI-backed module in this tool
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()
	rc := 0
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			rc = exitErr.ExitCode()
		} else {
			return Result{}, runErr
		}
	}

	return Result{
		RC:          rc,
		Stdout:      stdout.String(),
		StdoutLines: splitLines(stdout.String()),
		Stderr:      stderr.String(),
		StderrLines: splitLines(stderr.String()),
	}, nil
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return []string{}
	}
	return strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
}
