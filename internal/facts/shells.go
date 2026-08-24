package facts

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// versionProbeTimeout bounds every '--version' probe below (and pwsh's,
// see pwsh.go) - a bare interpreter name on PATH isn't necessarily a
// well-behaved one (e.g. a custom WSL-forwarding wrapper that ignores
// its args and drops into an interactive shell instead of printing a
// version and exiting - observed live with a personal zsh.exe wrapper
// during development). Facts gathering must never hang the whole run
// waiting on a misbehaving external command.
const versionProbeTimeout = 3 * time.Second

// {bash,zsh,fish,nu}Runner mirror pwshRunner (pwsh.go) - each overridable
// in tests, each reporting "what's on PATH", not "what's running this".
var bashRunner = func() (string, error) { return runVersionProbe("bash", "--version") }
var zshRunner = func() (string, error) { return runVersionProbe("zsh", "--version") }
var fishRunner = func() (string, error) { return runVersionProbe("fish", "--version") }
var nuRunner = func() (string, error) { return runVersionProbe("nu", "--version") }

// runVersionProbe resolves name on PATH and runs it with args, bounded by
// versionProbeTimeout and with stdin explicitly closed - a timeout (or
// any other error) is reported like "not found", not a fatal error, so
// one broken interpreter on PATH can't stop facts gathering or the rest
// of the run.
func runVersionProbe(name string, args ...string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), versionProbeTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...) //nolint:gosec // fixed argv, discovered via LookPath, no user input
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// firstLine trims s and keeps only its first line - some '--version'
// banners (bash's especially) are multi-line; only the summary line is
// useful as a fact value.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func versionFrom(runner func() (string, error)) string {
	out, err := runner()
	if err != nil {
		return ""
	}
	return firstLine(out)
}

func bashVersion() string { return versionFrom(bashRunner) }
func zshVersion() string  { return versionFrom(zshRunner) }
func fishVersion() string { return versionFrom(fishRunner) }
func nuVersion() string   { return versionFrom(nuRunner) }

// stringOrNil reports a version fact as null (rather than "") once it's
// absent - so 'facts.pwsh_version'/etc. are directly usable in a
// template without an extra 'is defined'/empty-string check.
func stringOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
