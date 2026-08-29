// Package facts gathers host facts made available to 'when' conditions
// and '${{ facts.* }}' templating — a port of modules/Facts.psm1's small,
// deliberately fixed starter set, plus 'platform'/'arch'/'os_family'
// (docs/plans/go-rewrite.md §1's cross-platform goal): ironstate.ps1 was
// always Windows-only, but the Go binary itself is genuinely
// cross-platform (windows/linux/darwin builds, see .goreleaser.yaml), so
// a main.yml can now usefully branch on 'when: facts.platform == "linux"'
// - a deliberate addition beyond parity, not a compatibility requirement.
// Gathered fresh every run (unlike 'vars', which come from YAML and are
// merged/overridable).
package facts

import (
	"os"
	"os/user"
	"runtime"
	"sync"
)

// Gather returns the fixed set of host facts as a map[string]any, ready
// to be merged under the 'facts' namespace (Common.psm1's
// Merge-FlatContext). Numbers use float64, matching internal/expr's
// numeric convention.
func Gather() map[string]any {
	shells := gatherShellVersions()
	return map[string]any{
		"computer_name": computerName(),
		"user_name":     userName(),
		"home":          homeDir(),
		"os_version":    osVersion(),
		"os_build":      float64(osBuildNumber()),
		"is_admin":      isAdmin(),
		"shell_pwsh":    shells.pwsh != "",
		"pwsh_version":  stringOrNil(shells.pwsh),
		"shell_bash":    shells.bash != "",
		"bash_version":  stringOrNil(shells.bash),
		"shell_zsh":     shells.zsh != "",
		"zsh_version":   stringOrNil(shells.zsh),
		"shell_fish":    shells.fish != "",
		"fish_version":  stringOrNil(shells.fish),
		"shell_nu":      shells.nu != "",
		"nu_version":    stringOrNil(shells.nu),
		"platform":      runtime.GOOS,
		"arch":          runtime.GOARCH,
		"os_family":     osFamily(runtime.GOOS),
	}
}

// shellVersionFacts holds every shell version probe's result, gathered
// concurrently by gatherShellVersions.
type shellVersionFacts struct {
	pwsh, bash, zsh, fish, nu string
}

// gatherShellVersions runs every '--version' probe (each independently
// bounded by versionProbeTimeout, see shells.go) in parallel, so a slow
// or misbehaving interpreter on PATH adds at most one timeout's worth of
// latency to Gather() instead of stacking one after another.
func gatherShellVersions() shellVersionFacts {
	var v shellVersionFacts
	var wg sync.WaitGroup
	probe := func(dst *string, fn func() string) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			*dst = fn()
		}()
	}
	probe(&v.pwsh, pwshVersion)
	probe(&v.bash, bashVersion)
	probe(&v.zsh, zshVersion)
	probe(&v.fish, fishVersion)
	probe(&v.nu, nuVersion)
	wg.Wait()
	return v
}

// osFamily identifies the host's OS for 'when' branching and chained
// override filenames (e.g. '<username>.ubuntu.yml', '<hostname>.archlinux.yml'):
// "windows"/"darwin" as-is, but on Linux it resolves to the specific
// distribution ID from distro() (e.g. "ubuntu", "archlinux") when
// detectable, falling back to the raw GOOS otherwise - distinct from
// 'platform', which always stays the plain GOOS value.
func osFamily(goos string) string {
	if goos == "linux" {
		if d := distro(); d != "" {
			return d
		}
	}
	return goos
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
