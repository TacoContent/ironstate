//go:build !windows && !linux && !darwin

package facts

// platformMounts keeps its mounts.go default (a "not supported" error) on
// this platform — this build exists purely so internal/facts (and its
// callers) compile for engine-only testing on unsupported CI runners
// (docs/plans/go-rewrite.md §4.6), mirroring facts_other.go.
