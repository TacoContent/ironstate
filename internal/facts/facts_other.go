//go:build !windows

package facts

// isAdmin/osVersion have no faithful equivalent off Windows — this build
// exists purely so internal/facts (and its callers) compile for
// engine-only testing on non-Windows CI runners (docs/plans/go-rewrite.md
// §4.6), not to give real answers on a non-Windows host.
func isAdmin() bool { return false }

func osVersion() (major, minor, build uint32) { return 0, 0, 0 }
