package facts

import (
	"os/exec"
	"strings"
)

// pwshRunner is overridable in tests.
var pwshRunner = func() (string, error) {
	path, err := exec.LookPath("pwsh")
	if err != nil {
		return "", err
	}
	out, err := exec.Command(path, "--version").Output() //nolint:gosec // fixed argv, discovered via LookPath, no user input
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// pwshVersion reports the newest PowerShell found on PATH, or "" if none
// — the runner itself no longer needs to *be* PowerShell, unlike
// ironstate.ps1 (which reports its own $PSVersionTable.PSVersion), so
// this is an intentional semantic shift: "what pwsh is available", not
// "what pwsh is this running under" (docs/plans/go-rewrite.md §4.6/§11).
func pwshVersion() string {
	out, err := pwshRunner()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}
