package handlers

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// zipHandler ports Handlers/Zip.psm1: downloads and extracts a ZIP
// archive directly. Idempotency is entirely 'creates'-glob based (shared
// with the future 'shell' handler, see creates.go) - Test never inspects
// the archive/dest contents itself.
type zipHandler struct{}

func zipSha256CachePath(item map[string]any) string {
	if sha256Spec := getMap(item, "sha256"); sha256Spec != nil {
		if cache := getString(sha256Spec, "cache"); cache != "" {
			return resolvePath(cache)
		}
	}
	dest := resolvePath(getString(item, "dest"))
	src := getString(item, "src")
	return filepath.Join(dest, path.Base(src)+".sha256")
}

func httpGetToFile(url, destPath string) error {
	resp, err := httpGet(url) //nolint:gosec // URL is authored YAML content, same trust boundary as the rest of this tool
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	f, err := os.Create(destPath) //nolint:gosec // destPath is a generated temp file path
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// httpGet is overridable so tests never make a real network call.
var httpGet = func(url string) (*http.Response, error) { return http.Get(url) } //nolint:gosec // URL is authored YAML content, same trust boundary as the rest of this tool

func sha256HexOfFile(path string) (string, error) {
	f, err := os.Open(path) //nolint:gosec // path is a generated temp file path
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New() //nolint:gosec // content-identity hash, not a security control
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func matchAny(patterns []any, name string) bool {
	if len(patterns) == 0 {
		return false
	}
	for _, raw := range patterns {
		pattern, _ := raw.(string)
		if pattern == "" {
			continue
		}
		if ok, _ := path.Match(pattern, name); ok {
			return true
		}
	}
	return false
}

func invokeZipDownloadAndExtract(item map[string]any) error {
	src := getString(item, "src")
	dest := resolvePath(getString(item, "dest"))
	state := itemState(item)
	include := asList(item["include"])
	exclude := asList(item["exclude"])

	if err := ensureParentDir(filepath.Join(dest, "placeholder")); err != nil {
		return err
	}

	cachePath := zipSha256CachePath(item)
	tempFile, err := os.CreateTemp("", "ironstate-*.zip")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	if err := tempFile.Close(); err != nil {
		return err
	}
	defer func() { _ = os.Remove(tempPath) }()

	if err := httpGetToFile(src, tempPath); err != nil {
		return err
	}

	newHash, err := sha256HexOfFile(tempPath)
	if err != nil {
		return err
	}

	if state == "latest" && fileExists(cachePath) {
		cached, err := os.ReadFile(cachePath) //nolint:gosec // cachePath is derived from authored YAML content
		if err == nil && strings.TrimSpace(string(cached)) == newHash {
			return nil
		}
	}

	r, err := zip.OpenReader(tempPath)
	if err != nil {
		return err
	}
	defer func() { _ = r.Close() }()

	for _, entry := range r.File {
		name := entry.Name
		if name == "" || strings.HasSuffix(name, "/") {
			continue
		}
		if len(include) > 0 && !matchAny(include, name) {
			continue
		}
		if len(exclude) > 0 && matchAny(exclude, name) {
			continue
		}
		if !isSafeZipEntryName(name) {
			engine.Danger("zip entry %q is unsafe; aborting extraction", name)
			return fmt.Errorf("zip entry %q is unsafe", name)
		}
		destPath, err := safeExtractPath(dest, name)
		if err != nil {
			engine.Danger("zip entry %q escapes destination %s; aborting extraction: %v", name, dest, err)
			return fmt.Errorf("zip entry %q escapes destination %s: %w", name, dest, err)
		}
		if err := extractZipEntry(entry, destPath); err != nil {
			return err
		}
	}

	if err := ensureParentDir(cachePath); err != nil {
		return err
	}
	return os.WriteFile(cachePath, []byte(newHash), 0o644) //nolint:gosec // matches the archive's own trust level, no tighter mode intended
}

// isSafeZipEntryName reports whether a raw zip entry name is safe to
// extract - no absolute path, no path-traversal ('..') segment. Checked
// as its own explicit guard directly against the untrusted zip entry
// name (not only inside safeExtractPath) so the "no zip-slip" check is
// visibly applied to the tainted source before any path is constructed
// from it.
func isSafeZipEntryName(name string) bool {
	if name == "" {
		return false
	}
	zipName := strings.ReplaceAll(name, "\\", "/")
	if strings.HasPrefix(zipName, "/") || filepath.IsAbs(zipName) {
		return false
	}
	cleanZipName := path.Clean(zipName)
	return cleanZipName != ".." && !strings.HasPrefix(cleanZipName, "../")
}

// safeExtractPath resolves a zip entry output path and enforces that the
// resulting absolute path stays within dest.
func safeExtractPath(dest, name string) (string, error) {
	if !isSafeZipEntryName(name) {
		return "", fmt.Errorf("illegal file path")
	}

	cleanZipName := path.Clean(strings.ReplaceAll(name, "\\", "/"))
	joined := filepath.Join(dest, cleanZipName)

	baseAbs, err := filepath.Abs(dest)
	if err != nil {
		return "", err
	}
	targetAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}

	baseAbs = filepath.Clean(baseAbs)
	targetAbs = filepath.Clean(targetAbs)
	baseWithSep := baseAbs + string(os.PathSeparator)
	if targetAbs != baseAbs && !strings.HasPrefix(targetAbs+string(os.PathSeparator), baseWithSep) {
		return "", fmt.Errorf("illegal file path")
	}
	return targetAbs, nil
}

func extractZipEntry(entry *zip.File, destPath string) error {
	if err := ensureParentDir(destPath); err != nil {
		return err
	}
	rc, err := entry.Open()
	if err != nil {
		return err
	}
	defer func() { _ = rc.Close() }()

	out, err := os.OpenFile(destPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644) //nolint:gosec // destPath is validated by safeJoin against zip-slip, same trust boundary as the rest of this tool
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, rc); err != nil { //nolint:gosec // archive contents are authored/trusted install content, same trust boundary as the rest of this tool
		_ = out.Close()
		return err
	}
	return out.Close()
}

func removeZipCreates(item map[string]any) {
	removeCreatesPatterns(asList(item["creates"]))
	cachePath := zipSha256CachePath(item)
	if fileExists(cachePath) {
		_ = os.Remove(cachePath)
	}
}

func (zipHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return testCreatesPresent(asList(item["creates"])), nil
}

func (zipHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	src := getString(item, "src")
	dest := resolvePath(getString(item, "dest"))
	if action == engine.ActionUninstall {
		return "remove creates entries for " + src + " -> " + dest, nil
	}
	return "download and extract " + src + " -> " + dest, nil
}

func (zipHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, invokeZipDownloadAndExtract(item)
}

func (zipHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	removeZipCreates(item)
	return engine.ExecResult{}, nil
}
