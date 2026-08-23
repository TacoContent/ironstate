// Package pathutil holds small filesystem-path helpers shared across
// internal/filters and (in later phases) internal/handlers — ports of
// Common.psm1's Resolve-UserPath / ConvertTo-NormalizedPathString and the
// 'join' filter's .NET Path.Combine semantics.
package pathutil

import (
	"os"
	"path/filepath"
	"strings"
)

// ResolveUserPath expands a leading '~' (or '~/'/'~\') to the current
// user's home directory, mirroring Common.psm1's Resolve-UserPath.
// Anything else passes through unchanged.
func ResolveUserPath(path string) string {
	if path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") || strings.HasPrefix(path, `~\`) {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// NormalizeSeparators mirrors Common.psm1's ConvertTo-NormalizedPathString:
// forward slashes become backslashes and a trailing separator is trimmed,
// so two strings referring to the same path compare equal regardless of
// slash direction (Windows always reports a real symlink's target with
// backslashes, regardless of how the YAML wrote it).
func NormalizeSeparators(path string) string {
	if path == "" {
		return path
	}
	return strings.TrimRight(strings.ReplaceAll(path, "/", `\`), `\`)
}

// CombinePaths mirrors .NET's [System.IO.Path]::Combine(parts): each part
// after the first that is itself rooted/absolute discards everything
// combined so far, rather than being appended onto it.
func CombinePaths(parts []string) string {
	result := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if filepath.IsAbs(p) || result == "" {
			result = p
			continue
		}
		result = filepath.Join(result, p)
	}
	return result
}
