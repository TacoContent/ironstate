package handlers

import (
	"os"
	"path/filepath"

	"github.com/TacoContent/ironstate/internal/pathutil"
)

// testCreatesPresent ports Common.psm1's Test-CreatesPresent: true only
// when every 'creates' pattern resolves to at least one existing path. An
// empty/absent 'creates' list means "can't tell" -> always not-installed -
// shared by 'zip' and (in Phase 4) 'shell'.
func testCreatesPresent(creates []any) bool {
	if len(creates) == 0 {
		return false
	}
	for _, raw := range creates {
		pattern, _ := raw.(string)
		if pattern == "" {
			return false
		}
		if !createsPatternMatches(pattern) {
			return false
		}
	}
	return true
}

func createsPatternMatches(pattern string) bool {
	resolved := pathutil.ResolveUserPath(pattern)
	if hasGlobMeta(resolved) {
		parent := filepath.Dir(resolved)
		leaf := filepath.Base(resolved)
		if !fileExists(parent) {
			return false
		}
		matches, _ := filepath.Glob(filepath.Join(parent, leaf))
		return len(matches) > 0
	}
	return fileExists(resolved)
}

// removeCreatesPatterns ports Common.psm1's Remove-CreatesPatterns.
func removeCreatesPatterns(creates []any) {
	for _, raw := range creates {
		pattern, _ := raw.(string)
		if pattern == "" {
			continue
		}
		resolved := pathutil.ResolveUserPath(pattern)
		if hasGlobMeta(resolved) {
			parent := filepath.Dir(resolved)
			leaf := filepath.Base(resolved)
			matches, _ := filepath.Glob(filepath.Join(parent, leaf))
			for _, m := range matches {
				_ = os.Remove(m)
			}
			continue
		}
		if fileExists(resolved) {
			_ = os.RemoveAll(resolved)
		}
	}
}

func hasGlobMeta(s string) bool {
	for _, r := range s {
		if r == '*' || r == '?' {
			return true
		}
	}
	return false
}
