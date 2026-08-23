package handlers

// Package handlers implements the Handler interface for every Phase 3
// module (docs/plans/go-rewrite.md §10): log, path, fact, assert, file,
// copy, symlinks, blockinfile, ssh_host_block, zip. Package-manager
// handlers (winget, chocolatey, ...), 'shell', 'template', 'registry', and
// 'scheduled_task' land in Phase 4.
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
	"winget", "chocolatey", "gem", "pipx", "npm", "cargo", "go", "eget",
	"zip", "symlinks", "file", "copy", "template", "shell", "blockinfile",
	"ssh_host_block", "log", "path", "fact", "registry", "scheduled_task",
	"assert",
}

// All returns every module implemented so far, ready to hand to
// engine.Options.Handlers. A leaf whose module isn't in this map yet
// (Phase 4's package managers, 'shell', 'template', 'registry',
// 'scheduled_task') is skipped with a warning by internal/engine's
// dispatch loop - not an error - matching ironstate.ps1's own "no handler
// registered" behavior for an unrecognized module.
func All() map[string]engine.Handler {
	return map[string]engine.Handler{
		"log":            logHandler{},
		"path":           pathHandler{},
		"fact":           factHandler{},
		"assert":         assertHandler{},
		"file":           fileHandler{},
		"copy":           copyHandler{},
		"symlinks":       symlinksHandler{},
		"blockinfile":    blockInFileHandler{},
		"ssh_host_block": sshHostBlockHandler{},
		"zip":            zipHandler{},
	}
}
