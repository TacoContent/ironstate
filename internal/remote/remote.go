package remote

// Package remote resolves a 'uses:' task's source - a git repository
// (ssh or https), a local directory, or a network share - to a real
// directory on this machine. Git sources are cloned once into a
// per-URL/ref cache directory and refreshed on later runs (unless the
// ref is a pinned commit SHA).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/TacoContent/ironstate/internal/pathutil"
)

// Kind classifies where a source's content comes from.
type Kind string

const (
	// KindLocal is a plain filesystem path or a UNC network share.
	KindLocal Kind = "local"
	// KindGit is a git repository reachable over ssh, https, or git://.
	KindGit Kind = "git"
)

// Spec is the source half of a 'uses:' task.
type Spec struct {
	// Remote is the repository URL, filesystem path, or UNC share.
	Remote string
	// Path is an optional sub-directory (or file) within Remote.
	Path string
	// Ref is an optional git branch, tag, or commit.
	Ref string
}

// Resolved is a Spec after the source has been made available locally.
type Resolved struct {
	// Dir is the directory the used document should be loaded from.
	Dir string
	// Kind is the source classification.
	Kind Kind
	// Source is a human-readable description of where Dir came from.
	Source string
}

// CloneTimeout bounds a single git clone/fetch.
var CloneTimeout = 2 * time.Minute

// CacheRoot returns the directory git clones are cached under.
// Overridable for tests.
var CacheRoot = func() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "ironstate", "remotes"), nil
}

// RunGit executes a git command in dir. Overridable for tests.
var RunGit = func(dir string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), CloneTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", args...) //nolint:gosec // args are validated by Classify/validateGitArg before reaching here
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader("")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Warn reports a non-fatal problem. Overridable for tests.
var Warn = func(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}

// commitSHA matches an abbreviated or full git commit hash.
var commitSHA = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)

var gitURLPrefixes = []string{"ssh://", "git://", "git+ssh://", "git+https://"}

// scpLikeGitURL matches git's scp-style syntax, e.g.
// "git@github.com:owner/repo.git".
var scpLikeGitURL = regexp.MustCompile(`^[A-Za-z0-9_.\-]+@[A-Za-z0-9_.\-]+:`)

// Classify decides whether remote names a git repository or a local
// path/network share.
func Classify(remote string) Kind {
	trimmed := strings.TrimSpace(remote)
	lower := strings.ToLower(trimmed)
	for _, p := range gitURLPrefixes {
		if strings.HasPrefix(lower, p) {
			return KindGit
		}
	}
	if scpLikeGitURL.MatchString(trimmed) {
		return KindGit
	}
	// A UNC share ("\\server\share\...") looks superficially like a URL
	// but is an ordinary filesystem path.
	if strings.HasPrefix(trimmed, `\\`) || strings.HasPrefix(trimmed, "//") {
		return KindLocal
	}
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		if strings.HasSuffix(strings.TrimSuffix(lower, "/"), ".git") {
			return KindGit
		}
	}
	return KindLocal
}

// Describe classifies spec's source and renders a human-readable
// description of it WITHOUT touching the network or filesystem - so the
// operator can be asked whether to trust a remote source before any of
// its code is fetched.
func Describe(spec Spec) (Kind, string) {
	remote := strings.TrimSpace(spec.Remote)
	source := remote
	if ref := strings.TrimSpace(spec.Ref); ref != "" {
		source += "@" + ref
	}
	if spec.Path != "" {
		source += " (" + spec.Path + ")"
	}
	return Classify(remote), source
}

// Resolve makes spec's source available locally and returns the
// directory the source should be loaded from. baseDir anchors a relative
// local path (normally the consuming playbook's own directory).
//
// For a remote source this performs a real network fetch - call
// Describe/NeedsConfirmation and obtain the operator's approval first.
func Resolve(spec Spec, baseDir string) (*Resolved, error) {
	remote := strings.TrimSpace(spec.Remote)
	if remote == "" {
		return nil, fmt.Errorf("uses has no 'remote'")
	}
	kind, source := Describe(spec)

	var root string
	switch kind {
	case KindLocal:
		root = pathutil.ResolveUserPath(remote)
		if !filepath.IsAbs(root) && baseDir != "" {
			root = filepath.Join(baseDir, root)
		}
		fi, err := os.Stat(root) //nolint:gosec // an operator-configured playbook location, same trust boundary as --playbook itself
		if err != nil {
			return nil, fmt.Errorf("uses source not found: %s: %w", root, err)
		}
		if !fi.IsDir() {
			return nil, fmt.Errorf("uses source is not a directory: %s", root)
		}
	case KindGit:
		dir, err := fetchGit(remote, strings.TrimSpace(spec.Ref))
		if err != nil {
			return nil, err
		}
		root = dir
	}

	dir, err := safeSubPath(root, spec.Path)
	if err != nil {
		return nil, err
	}
	return &Resolved{Dir: dir, Kind: kind, Source: source}, nil
}

// safeSubPath joins sub onto root, rejecting any value that would escape
// root (a "../.." sub-path, an absolute path, or a symlink-free
// traversal).
func safeSubPath(root, sub string) (string, error) {
	sub = strings.TrimSpace(sub)
	if sub == "" {
		return root, nil
	}
	normalized := strings.ReplaceAll(sub, `\`, "/")
	if strings.HasPrefix(normalized, "/") || filepath.IsAbs(sub) {
		return "", fmt.Errorf("uses 'path' must be relative to the source: %q", sub)
	}
	cleaned := filepath.Clean(filepath.FromSlash(normalized))
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("uses 'path' escapes the source: %q", sub)
	}
	joined := filepath.Join(root, cleaned)
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	absJoined, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	if absJoined != absRoot && !strings.HasPrefix(absJoined, absRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("uses 'path' escapes the source: %q", sub)
	}
	return absJoined, nil
}

// validateGitArg rejects a value that git would interpret as an option
// rather than a URL/ref.
func validateGitArg(kind, value string) error {
	if strings.HasPrefix(value, "-") {
		return fmt.Errorf("uses %s may not start with '-': %q", kind, value)
	}
	return nil
}

// fetchGit clones remote@ref into the cache and returns the checkout
// directory. An existing checkout is refreshed to the latest ref (a
// pinned commit SHA is reused as-is); if the refresh fails, the cached
// copy is used and a warning is emitted.
func fetchGit(remote, ref string) (string, error) {
	if err := validateGitArg("'remote'", remote); err != nil {
		return "", err
	}
	if err := validateGitArg("'ref'", ref); err != nil {
		return "", err
	}
	root, err := CacheRoot()
	if err != nil {
		return "", fmt.Errorf("resolving remote cache directory: %w", err)
	}
	dest := filepath.Join(root, cacheKey(remote, ref))
	if fi, err := os.Stat(dest); err == nil && fi.IsDir() { //nolint:gosec // cache path derived from a hash, not user input
		refreshGit(dest, remote, ref)
		return dest, nil
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return "", err
	}

	args := []string{"clone", "--depth", "1"}
	if ref != "" {
		args = append(args, "--branch", ref)
	}
	args = append(args, "--", remote, dest)
	if err := RunGit(root, args...); err != nil {
		if ref == "" {
			return "", err
		}
		// '--branch' only accepts a branch or tag; fall back to a full
		// clone + checkout so a commit SHA also works.
		_ = os.RemoveAll(dest)
		if err := RunGit(root, "clone", "--", remote, dest); err != nil {
			return "", err
		}
		if err := RunGit(dest, "checkout", "--detach", ref); err != nil {
			_ = os.RemoveAll(dest)
			return "", err
		}
	}
	return dest, nil
}

// refreshGit updates an existing cached checkout to the tip of ref.
func refreshGit(dest, remote, ref string) {
	if commitSHA.MatchString(ref) {
		return
	}
	target := ref
	if target == "" {
		target = "HEAD"
	}
	if err := RunGit(dest, "fetch", "--depth", "1", "origin", "--", target); err != nil {
		Warn("could not refresh %s@%s, using cached copy: %v", remote, target, err)
		return
	}
	if err := RunGit(dest, "reset", "--hard", "FETCH_HEAD"); err != nil {
		Warn("could not update cached %s@%s, using cached copy: %v", remote, target, err)
	}
}

// cacheKey derives a stable, filesystem-safe directory name for a
// remote+ref pair, keeping a readable prefix for humans browsing the
// cache.
func cacheKey(remote, ref string) string {
	sum := sha256.Sum256([]byte(remote + "\x00" + ref))
	digest := hex.EncodeToString(sum[:])[:16]
	return sanitizeName(remote) + "-" + digest
}

var unsafeNameChars = regexp.MustCompile(`[^A-Za-z0-9_.-]+`)

func sanitizeName(remote string) string {
	base := remote
	if i := strings.LastIndexAny(base, "/:"); i >= 0 {
		base = base[i+1:]
	}
	base = strings.TrimSuffix(base, ".git")
	base = unsafeNameChars.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")
	if base == "" {
		base = "source"
	}
	if len(base) > 40 {
		base = base[:40]
	}
	return base
}
