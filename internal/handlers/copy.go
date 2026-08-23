package handlers

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// copyHandler ports Handlers/Copy.psm1: copies a local file - or a whole
// directory, recursively - into place. 'src' is already an absolute path
// by the time this handler runs (internal/packages.ResolveRelativePathsInPlace
// resolved it at load time).
type copyHandler struct{}

func copySrcIsDirectory(src string) bool {
	info, err := os.Stat(src)
	return err == nil && info.IsDir()
}

// copyDestRoot ports Get-CopyDestRoot: a trailing '/'/'\' on src copies
// its *contents* into dest directly; no trailing slash nests it as
// 'dest/<src's own folder name>/...'.
func copyDestRoot(src, dest string) string {
	if strings.HasSuffix(src, "/") || strings.HasSuffix(src, `\`) {
		return dest
	}
	return filepath.Join(dest, filepath.Base(strings.TrimRight(src, `/\`)))
}

// copyRelativeFiles lists every file under src, as paths relative to src.
func copyRelativeFiles(src string) ([]string, error) {
	root := strings.TrimRight(src, `/\`)
	var rel []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		r, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = append(rel, r)
		return nil
	})
	return rel, err
}

func testCopyDirectoryPresent(src, destRoot string) bool {
	relFiles, err := copyRelativeFiles(src)
	if err != nil {
		return false
	}
	if len(relFiles) == 0 {
		return fileExists(destRoot)
	}
	root := strings.TrimRight(src, `/\`)
	for _, rel := range relFiles {
		destFile := filepath.Join(destRoot, rel)
		if !fileExists(destFile) {
			return false
		}
		if !filesHaveSameHash(filepath.Join(root, rel), destFile) {
			return false
		}
	}
	return true
}

func installCopyDirectory(src, destRoot string) error {
	if !fileExists(destRoot) {
		if err := os.MkdirAll(destRoot, 0o755); err != nil { //nolint:gosec // matches ironstate.ps1's own directories, no tighter mode intended
			return err
		}
	}
	root := strings.TrimRight(src, `/\`)
	relFiles, err := copyRelativeFiles(src)
	if err != nil {
		return err
	}
	for _, rel := range relFiles {
		destFile := filepath.Join(destRoot, rel)
		if err := ensureParentDir(destFile); err != nil {
			return err
		}
		if err := copyFileContents(filepath.Join(root, rel), destFile); err != nil {
			return err
		}
	}
	return nil
}

// uninstallCopyDirectory removes only the files src would have copied,
// then prunes any subdirectories under destRoot left empty by that -
// never destRoot itself.
func uninstallCopyDirectory(src, destRoot string) error {
	relFiles, err := copyRelativeFiles(src)
	if err != nil {
		return err
	}
	touchedDirs := map[string]bool{}
	for _, rel := range relFiles {
		destFile := filepath.Join(destRoot, rel)
		if fileExists(destFile) {
			if err := os.Remove(destFile); err != nil {
				return err
			}
		}
		parent := filepath.Dir(destFile)
		for len(parent) > len(destRoot) {
			touchedDirs[parent] = true
			parent = filepath.Dir(parent)
		}
	}
	dirs := make([]string, 0, len(touchedDirs))
	for d := range touchedDirs {
		dirs = append(dirs, d)
	}
	// Deepest first, so a now-empty child is removed before its parent is checked.
	for i := 0; i < len(dirs); i++ {
		for j := i + 1; j < len(dirs); j++ {
			if len(dirs[j]) > len(dirs[i]) {
				dirs[i], dirs[j] = dirs[j], dirs[i]
			}
		}
	}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(dir)
		}
	}
	return nil
}

func copyFileContents(src, dest string) error {
	data, err := os.ReadFile(src) //nolint:gosec // src is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return err
	}
	return os.WriteFile(dest, data, 0o644) //nolint:gosec // matches source file's own trust level, no tighter mode intended
}

func (copyHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if !fileExists(src) {
		engine.Warn("Source path for copy does not exist: %s", src)
		return false, nil
	}
	if copySrcIsDirectory(src) {
		return testCopyDirectoryPresent(src, copyDestRoot(src, dest)), nil
	}
	if !fileExists(dest) {
		return false, nil
	}
	return filesHaveSameHash(src, dest), nil
}

func (copyHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if action == engine.ActionUninstall {
		return "remove " + dest, nil
	}
	return "copy " + src + " -> " + dest, nil
}

func (copyHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if !fileExists(src) {
		engine.Warn("Source path for copy does not exist, skipping: %s", src)
		return engine.ExecResult{}, nil
	}
	if copySrcIsDirectory(src) {
		return engine.ExecResult{}, installCopyDirectory(src, copyDestRoot(src, dest))
	}
	if err := ensureParentDir(dest); err != nil {
		return engine.ExecResult{}, err
	}
	return engine.ExecResult{}, copyFileContents(src, dest)
}

func (copyHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	if src != "" && copySrcIsDirectory(src) {
		return engine.ExecResult{}, uninstallCopyDirectory(src, copyDestRoot(src, dest))
	}
	if fileExists(dest) {
		return engine.ExecResult{}, os.Remove(dest)
	}
	return engine.ExecResult{}, nil
}
