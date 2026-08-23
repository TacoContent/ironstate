// Package packages implements YAML document loading, the site/host/user
// file-hierarchy merge, and 'include:' resolution — a port of
// modules/Packages.psm1.
package packages

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/TacoContent/ironstate/internal/model"
)

var urlOrHomePrefix = regexp.MustCompile(`^(https?://|~)`)

// resolveInstallRelativePath resolves a 'copy.src'/'shell.script'/
// 'template.src' path against baseDir, the directory owning the YAML
// file it came from. URLs, '~' paths, and already-rooted paths pass
// through untouched — ports Common.psm1's Resolve-InstallRelativePath.
func resolveInstallRelativePath(path, baseDir string) string {
	if urlOrHomePrefix.MatchString(path) {
		return path
	}
	if isPathRooted(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

// isPathRooted mirrors [System.IO.Path]::IsPathRooted rather than Go's
// stricter filepath.IsAbs: true for anything starting with '/' or '\'
// (rooted relative to the current drive, no drive letter required) or
// carrying a drive-letter prefix like "C:" — NOT the same test as "is a
// fully qualified absolute path". A YAML author's '/already/absolute'
// (Unix-style, no drive letter) must still be left untouched on Windows,
// exactly like the original PowerShell.
func isPathRooted(path string) bool {
	if path == "" {
		return false
	}
	if path[0] == '/' || path[0] == '\\' {
		return true
	}
	return len(path) >= 2 && path[1] == ':'
}

// ResolveRelativePathsInPlace rewrites 'copy.src', 'template.src',
// 'blockinfile.template.src', and 'shell.script' (including each
// present/absent/latest per-state block's own 'script') fields in-place
// from install-relative to absolute, immediately after a YAML file is
// loaded — ports Common.psm1's Resolve-RelativePathsInPlace. Only
// recurses through nested 'actions' (never 'include': an included
// package resolves its own paths against its own directory when it is
// itself loaded).
func ResolveRelativePathsInPlace(doc any, baseDir string) {
	taskList, _ := model.TaskList(doc)
	resolveInTaskList(taskList, baseDir)
}

func resolveInTaskList(tasks []any, baseDir string) {
	for _, raw := range tasks {
		item, ok := raw.(map[string]any)
		if !ok {
			continue
		}

		if actions, ok := item["actions"]; ok {
			resolveInTaskList(model.AsList(actions), baseDir)
			continue
		}

		if copySpec, ok := model.Prop(item, "copy"); ok {
			resolveFieldPath(model.AsMap(copySpec), "src", baseDir)
		}

		if tplSpec, ok := model.Prop(item, "template"); ok {
			resolveFieldPath(model.AsMap(tplSpec), "src", baseDir)
		}

		if blockSpec, ok := model.Prop(item, "blockinfile"); ok {
			if tpl, ok := model.Prop(blockSpec, "template"); ok {
				resolveFieldPath(model.AsMap(tpl), "src", baseDir)
			}
		}

		if shellSpec, ok := model.Prop(item, "shell"); ok {
			shellMap := model.AsMap(shellSpec)
			resolveFieldPath(shellMap, "script", baseDir)
			for _, state := range []string{"present", "absent", "latest"} {
				if block, ok := shellMap[state]; ok {
					resolveFieldPath(model.AsMap(block), "script", baseDir)
				}
			}
		}
	}
}

func resolveFieldPath(m map[string]any, key, baseDir string) {
	v, ok := m[key]
	if !ok {
		return
	}
	s, ok := v.(string)
	if !ok || strings.TrimSpace(s) == "" {
		return
	}
	m[key] = resolveInstallRelativePath(s, baseDir)
}
