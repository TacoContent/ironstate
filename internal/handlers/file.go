package handlers

import (
	"crypto/sha256"
	"io"
	"os"
	"time"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/pathutil"
)

// fileHandler ports Handlers/File.psm1: manages a path as a plain file,
// directory, symlink, or hard link. 'state' stays this codebase's usual
// present/absent/latest machine - NOT Ansible's overloaded 'file' module
// 'state' - so what *kind* of thing to manage is the separate 'type'
// field (file|directory|link|hard|touch, default 'file').
type fileHandler struct{}

// filePathKind classifies what already exists at path: "missing", "link"
// (symlink), "hard" (hard link - best-effort: Go's stdlib has no portable
// hardlink detection, so a hard link that isn't a symlink is reported as
// "file", same practical effect as a normal file for our Test purposes,
// see testFileItemPresent's 'hard' case which hashes instead of relying on
// link-kind detection), "directory", or "file".
func filePathKind(path string) string {
	info, err := os.Lstat(path)
	if err != nil {
		return "missing"
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "link"
	}
	if info.IsDir() {
		return "directory"
	}
	return "file"
}

func removeFileItemAtPath(path, kind string) error {
	if kind == "directory" {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func testFileItemPresent(item map[string]any) bool {
	path := resolvePath(getString(item, "path"))
	fileType := getStringOr(item, "type", "file")

	if fileType == "touch" {
		return false // always fires, like log/fact
	}

	kind := filePathKind(path)
	switch fileType {
	case "directory":
		return kind == "directory"
	case "link":
		if kind != "link" {
			return false
		}
		target, err := os.Readlink(path)
		if err != nil {
			return false
		}
		wantSrc := pathutil.NormalizeSeparators(resolvePath(getString(item, "src")))
		return pathutil.NormalizeSeparators(target) == wantSrc
	case "hard":
		src := resolvePath(getString(item, "src"))
		if !fileExists(src) || !fileExists(path) {
			return false
		}
		return filesHaveSameHash(path, src)
	default:
		return kind == "file"
	}
}

func filesHaveSameHash(a, b string) bool {
	ha, err := fileSHA256(a)
	if err != nil {
		return false
	}
	hb, err := fileSHA256(b)
	if err != nil {
		return false
	}
	return ha == hb
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New() //nolint:gosec // content-identity hash for idempotency comparison, not a security control
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return string(h.Sum(nil)), nil
}

func installFileItem(item map[string]any) error {
	path := resolvePath(getString(item, "path"))
	fileType := getStringOr(item, "type", "file")
	force := getBool(item, "force", false)

	if err := ensureParentDir(path); err != nil {
		return err
	}

	if fileType == "touch" {
		if !fileExists(path) {
			f, err := os.Create(path) //nolint:gosec // path is authored YAML content, same trust boundary as the rest of this tool
			if err != nil {
				return err
			}
			return f.Close()
		}
		now := time.Now()
		return os.Chtimes(path, now, now)
	}

	var src string
	if fileType == "link" || fileType == "hard" {
		src = resolvePath(getString(item, "src"))
		if !fileExists(src) {
			engine.Warn("Source path for '%s' does not exist, skipping: %s", fileType, src)
			return nil
		}
	}

	if testFileItemPresent(item) {
		return nil
	}

	kind := filePathKind(path)
	if kind != "missing" {
		if !force {
			engine.Warn("path already exists as something else, skipping (set force: true to replace): %s", path)
			return nil
		}
		if err := removeFileItemAtPath(path, kind); err != nil {
			return err
		}
	}

	switch fileType {
	case "directory":
		return os.MkdirAll(path, 0o755) //nolint:gosec // matches ironstate.ps1's own directories, no tighter mode intended
	case "link":
		return os.Symlink(src, path)
	case "hard":
		return os.Link(src, path)
	default:
		f, err := os.Create(path) //nolint:gosec // path is authored YAML content, same trust boundary as the rest of this tool
		if err != nil {
			return err
		}
		return f.Close()
	}
}

func uninstallFileItem(item map[string]any) error {
	path := resolvePath(getString(item, "path"))
	if !fileExists(path) {
		return nil
	}
	return removeFileItemAtPath(path, filePathKind(path))
}

func (fileHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testFileItemPresent(item), nil
}

func (fileHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	path := resolvePath(getString(item, "path"))
	fileType := getStringOr(item, "type", "file")
	if action == engine.ActionUninstall {
		return "remove " + path, nil
	}
	switch fileType {
	case "link":
		return "link " + path + " -> " + getString(item, "src"), nil
	case "hard":
		return "hard link " + path + " -> " + getString(item, "src"), nil
	default:
		return "ensure " + fileType + " " + path, nil
	}
}

func (fileHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, installFileItem(item)
}

func (fileHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, uninstallFileItem(item)
}
