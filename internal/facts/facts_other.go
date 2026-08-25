//go:build !windows && !linux && !darwin

package facts

// isAdmin/osVersion have no faithful equivalent on this platform — this
// build exists purely so internal/facts (and its callers) compile for
// engine-only testing on unsupported CI runners (docs/plans/go-rewrite.md
// §4.6), not to give real answers on a platform we don't build releases
// for. See facts_linux.go/facts_darwin.go for the real implementations.
func isAdmin() bool { return false }

func osVersion() string { return "" }

func osBuildNumber() uint32 { return 0 }
