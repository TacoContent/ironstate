package handlers

// Package handlers implements the Handler interface for every module
// implemented so far (docs/plans/go-rewrite.md §10): log, path, fact,
// assert, file, copy, symlinks, blockinfile, ssh_host_block, zip (Phase 3);
// winget, chocolatey, homebrew, apt, pacman, yum, apk, snap, flatpak,
// scoop, macports, pipx, npm, cargo, go, gem, eget,
// shell, registry, scheduled_task, template (Phase 4).
//
// Deviation from the master plan's §3 layout (one Go package per module
// under internal/handlers/<module>/): these Phase 3 handlers share a lot
// of small helpers (file-kind checks, blockinfile markers, the
// 'creates'-glob primitive) that are simplest to keep unexported within a
// single package rather than exporting them across a dozen subpackages
// for no real isolation benefit. Revisit if a later phase's handler count
// makes one flat package unwieldy.

import "github.com/TacoContent/ironstate/internal/engine"

// AllModuleNames lists every module key ironstate.ps1's
// Get-PackageManagerHandlers recognizes structurally (docs/plans/
// go-rewrite.md §2) - used for internal/tasks.Options.ModuleNames so
// flattening recognizes every leaf shape even before every module has a
// registered Handler (Phase 4 fills in the rest). Order matches
// internal/tasks/realfixture_test.go's realModuleNames.
var AllModuleNames = []string{
	"winget", "chocolatey", "homebrew", "brew", "apt", "pacman", "yum", "apk", "snap", "flatpak", "scoop", "macports", "gem", "pipx", "npm", "cargo", "go", "eget",
	"git", "cron", "cron_unix", "cron_file", "iptables", "ufw", "advfirewall", "firewall", "zip", "symlinks", "file", "copy", "template", "shell", "blockinfile", "lineinfile",
	"ssh_host_block", "log", "fail", "path", "fact", "mount_facts", "registry", "scheduled_task", "group", "user",
	"assert", "async", "wait_for", "service",
}

// All returns every implemented module, ready to hand to
// engine.Options.Handlers. A leaf whose module isn't in this map (there
// are none left unimplemented as of Phase 4) is skipped with a warning by
// internal/engine's dispatch loop - not an error - matching
// ironstate.ps1's own "no handler registered" behavior for an
// unrecognized module.
func All() map[string]engine.Handler {
	return map[string]engine.Handler{
		"log":            logHandler{},
		"fail":           failHandler{},
		"path":           pathHandler{},
		"fact":           factHandler{},
		"mount_facts":    mountFactsHandler{},
		"assert":         assertHandler{},
		"file":           fileHandler{},
		"copy":           copyHandler{},
		"symlinks":       symlinksHandler{},
		"blockinfile":    blockInFileHandler{},
		"ssh_host_block": sshHostBlockHandler{},
		"zip":            zipHandler{},
		"winget":         wingetHandler{},
		"chocolatey":     chocolateyHandler{},
		"homebrew":       homebrewHandler{},
		"brew":           homebrewHandler{}, // alias for 'homebrew' - both dispatch to the same handler
		"apt":            aptHandler{},
		"pacman":         pacmanHandler{},
		"yum":            yumHandler{},
		"apk":            apkHandler{},
		"snap":           snapHandler{},
		"flatpak":        flatpakHandler{},
		"scoop":          scoopHandler{},
		"macports":       macportsHandler{},
		"pipx":           pipxHandler{},
		"npm":            npmHandler{},
		"cargo":          cargoHandler{},
		"go":             goHandler{},
		"gem":            rubyGemHandler{},
		"eget":           egetHandler{},
		"git":            gitHandler{},
		"cron":           cronHandler{},
		"cron_unix":      cronUnixHandler{},
		"cron_file":      cronFileHandler{},
		"iptables":       iptablesHandler{},
		"ufw":            ufwHandler{},
		"advfirewall":    advFirewallHandler{},
		"firewall":       firewallHandler{},
		"shell":          shellHandler{},
		"registry":       registryHandler{},
		"scheduled_task": scheduledTaskHandler{},
		"group":          groupHandler{},
		"user":           userHandler{},
		"service":        serviceHandler{},
		"template":       templateHandler{},
		"lineinfile":     lineInFileHandler{},
		"async":          asyncHandler{},
		"wait_for":       waitForHandler{},
	}
}
