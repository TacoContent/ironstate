package facts

import "strings"

// pwshRunner is overridable in tests.
var pwshRunner = func() (string, error) { return runVersionProbe("pwsh", "--version") }

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
