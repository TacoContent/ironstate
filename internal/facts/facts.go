// Package facts gathers host facts made available to 'when' conditions
// and '${{ facts.* }}' templating — a port of modules/Facts.psm1's small,
// deliberately fixed starter set, plus 'platform'/'arch'/'os_family'
// (docs/plans/go-rewrite.md §1's cross-platform goal): ironstate.ps1 was
// always Windows-only, but the Go binary itself is genuinely
// cross-platform (windows/linux/darwin builds, see .goreleaser.yaml), so
// a site.yml can now usefully branch on 'when: facts.platform == "linux"'
// - a deliberate addition beyond parity, not a compatibility requirement.
// Gathered fresh every run (unlike 'vars', which come from YAML and are
// merged/overridable).
package facts

import (
	"os"
	"os/user"
	"runtime"
)

// Gather returns the fixed set of host facts as a map[string]any, ready
// to be merged under the 'facts' namespace (Common.psm1's
// Merge-FlatContext). Numbers use float64, matching internal/expr's
// numeric convention.
func Gather() map[string]any {
	major, minor, build := osVersion()
	return map[string]any{
		"computer_name": computerName(),
		"user_name":     userName(),
		"home":          homeDir(),
		"os_version":    formatVersion(major, minor, build),
		"os_build":      float64(build),
		"is_admin":      isAdmin(),
		"pwsh_version":  pwshVersion(),
		"platform":      runtime.GOOS,
		"arch":          runtime.GOARCH,
		"os_family":     osFamily(runtime.GOOS),
	}
}

// osFamily groups a Go GOOS value into a broader category useful for
// 'when' branching that only cares "Windows or POSIX-like", not the
// exact OS - distinct from 'platform', which is the precise GOOS value.
func osFamily(goos string) string {
	switch goos {
	case "windows":
		return "windows"
	case "linux", "darwin", "freebsd", "openbsd", "netbsd", "dragonfly", "solaris", "aix":
		return "unix"
	default:
		return goos
	}
}

func computerName() string {
	if v := os.Getenv("COMPUTERNAME"); v != "" {
		return v
	}
	if h, err := os.Hostname(); err == nil {
		return h
	}
	return ""
}

func userName() string {
	if v := os.Getenv("USERNAME"); v != "" {
		return v
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return os.Getenv("USER")
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return ""
}

func formatVersion(major, minor, build uint32) string {
	return itoa(major) + "." + itoa(minor) + "." + itoa(build)
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var digits [10]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}
