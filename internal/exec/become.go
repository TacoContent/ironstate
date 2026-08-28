package exec

import (
	"errors"
	"os/exec"
	"strings"
)

// Become describes a leaf's requested privilege elevation - the
// normalized form of the 'become' task property (bool or a target
// username, see tasks.Leaf.Become / engine's resolveBecome):
// 'become: true' elevates to the platform's default super-user,
// 'become: "someuser"' elevates to that specific user, 'become: false'
// (or the field being absent) requests no elevation.
type Become struct {
	Enabled bool
	User    string // "" (or "root") means the platform's default elevated identity
}

// current is the ambient elevation directive for whichever leaf is
// currently dispatching. Leaves dispatch strictly sequentially (see
// internal/engine.RunLeaves), so a package-level var is safe here - the
// same "swappable global describing the current operation" pattern this
// codebase already uses for internal/engine's Info/Warn/Danger/LookPath.
// internal/engine sets this immediately before calling a handler's
// Install/Uninstall and clears it right after, so it never leaks into an
// unrelated leaf's commands. This lets 'become' apply to every CLI-backed
// handler's runExternalCommand call automatically, without threading a
// Become parameter through every handler's Install/Uninstall signature.
var current Become

// SetBecome sets the ambient elevation directive for commands run via
// WrapForBecome until the next SetBecome/ClearBecome call.
func SetBecome(b Become) { current = b }

// ClearBecome resets the ambient elevation directive to "no elevation".
func ClearBecome() { current = Become{} }

// CurrentBecome returns the ambient elevation directive.
func CurrentBecome() Become { return current }

// LookSudoPath resolves 'sudo' on PATH - overridable for tests, matching
// engine.LookPath's own pattern for the same reason (never depend on the
// test host's real PATH). On Windows this resolves Windows 11's built-in
// sudo.exe (Settings > For developers > Enable sudo) the same way.
var LookSudoPath = func() (string, error) { return exec.LookPath("sudo") }

// WrapForBecome prepends a 'sudo' invocation ahead of exe/args when b
// requests elevation. Both Unix sudo and Windows' sudo.exe may prompt
// interactively (a password, or a UAC dialog) when invoked this way -
// that's expected, not a bug, since 'become' is meant for interactive
// use, not headless automation. Returns an error - never a silent
// fallback to running unelevated - when 'sudo' isn't on PATH at all, so a
// requested-but-unavailable elevation fails the leaf the same way any
// other command-not-found does, per the "elevation failure fails the
// task" contract.
func WrapForBecome(b Become, exe string, args []string) (string, []string, error) {
	if !b.Enabled {
		return exe, args, nil
	}
	sudoPath, err := LookSudoPath()
	if err != nil {
		return "", nil, errors.New("become requested but 'sudo' was not found on PATH")
	}
	wrapped := make([]string, 0, len(args)+3)
	if b.User != "" && !strings.EqualFold(b.User, "root") {
		wrapped = append(wrapped, "-u", b.User)
	}
	wrapped = append(wrapped, exe)
	wrapped = append(wrapped, args...)
	return sudoPath, wrapped, nil
}
