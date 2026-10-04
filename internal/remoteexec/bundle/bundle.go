// Package bundle packs the files a remote run needs into a tar.gz stream
// and safely unpacks it on the target (docs/plans/remote-apply.md §7).
package bundle

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// IgnoreFileName is read from each bundled directory's root.
const IgnoreFileName = ".ironstateignore"

// MaxExtractBytes bounds the total uncompressed size Extract will write.
const MaxExtractBytes int64 = 4 << 30

// alwaysExcluded names are never bundled, at any depth. Env files travel
// in the job header instead of on disk.
var alwaysExcluded = map[string]bool{".git": true, ".env": true, ".secrets": true}

// Dir bundles a directory tree under Dest.
type Dir struct {
	Source string
	Dest   string
}

// File bundles a single file at Dest.
type File struct {
	Source string
	Dest   string
}

// Spec lists everything to pack. Dest paths are slash-separated and
// relative to the bundle root.
type Spec struct {
	Dirs      []Dir
	Files     []File
	Generated map[string][]byte
}

// Info describes a built bundle.
type Info struct {
	Size   int64
	SHA256 string
}

type countingWriter struct {
	w io.Writer
	n int64
}

func (c *countingWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// Build writes spec as tar.gz to w.
func Build(w io.Writer, spec Spec) (Info, error) {
	hash := sha256.New()
	counter := &countingWriter{w: io.MultiWriter(w, hash)}
	gz := gzip.NewWriter(counter)
	tw := tar.NewWriter(gz)

	for _, d := range spec.Dirs {
		if err := addDir(tw, d); err != nil {
			return Info{}, err
		}
	}
	for _, f := range spec.Files {
		if err := checkDest(f.Dest); err != nil {
			return Info{}, err
		}
		info, err := os.Stat(f.Source)
		if err != nil {
			return Info{}, err
		}
		if err := addRegular(tw, f.Source, f.Dest, info); err != nil {
			return Info{}, err
		}
	}
	names := make([]string, 0, len(spec.Generated))
	for name := range spec.Generated {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := checkDest(name); err != nil {
			return Info{}, err
		}
		data := spec.Generated[name]
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), Typeflag: tar.TypeReg, ModTime: time.Unix(0, 0)}
		if err := tw.WriteHeader(hdr); err != nil {
			return Info{}, err
		}
		if _, err := tw.Write(data); err != nil {
			return Info{}, err
		}
	}

	if err := tw.Close(); err != nil {
		return Info{}, err
	}
	if err := gz.Close(); err != nil {
		return Info{}, err
	}
	return Info{Size: counter.n, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func checkDest(dest string) error {
	if !IsSafeEntryName(dest) {
		return fmt.Errorf("invalid bundle destination %q", dest)
	}
	return nil
}

func addDir(tw *tar.Writer, d Dir) error {
	if err := checkDest(d.Dest); err != nil {
		return err
	}
	patterns, err := LoadIgnoreFile(filepath.Join(d.Source, IgnoreFileName))
	if err != nil {
		return err
	}
	return filepath.WalkDir(d.Source, func(p string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(d.Source, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return writeDirHeader(tw, d.Dest)
		}
		if alwaysExcluded[entry.Name()] || matchesIgnore(patterns, rel, entry.IsDir()) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := path.Join(d.Dest, rel)
		switch {
		case entry.Type()&fs.ModeSymlink != 0:
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			target = filepath.ToSlash(target)
			if !safeLink(name, target) {
				return fmt.Errorf("symlink %s -> %s points outside the bundle; replace it with a copy or add it to %s", p, target, IgnoreFileName)
			}
			return tw.WriteHeader(&tar.Header{Name: name, Typeflag: tar.TypeSymlink, Linkname: target, Mode: 0o777, ModTime: time.Unix(0, 0)})
		case entry.IsDir():
			return writeDirHeader(tw, name)
		case entry.Type().IsRegular():
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return addRegular(tw, p, name, info)
		default:
			return nil
		}
	})
}

func writeDirHeader(tw *tar.Writer, name string) error {
	return tw.WriteHeader(&tar.Header{Name: name + "/", Typeflag: tar.TypeDir, Mode: 0o700, ModTime: time.Unix(0, 0)})
}

func addRegular(tw *tar.Writer, source, name string, info fs.FileInfo) error {
	mode := int64(0o600)
	if info.Mode().Perm()&0o111 != 0 {
		mode = 0o700
	}
	hdr := &tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: mode, Size: info.Size(), ModTime: time.Unix(0, 0)}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	f, err := os.Open(source) //nolint:gosec // walking the operator's own playbook tree
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(tw, f)
	return err
}

// LoadIgnoreFile reads glob patterns (one per line, '#' comments). A
// trailing '/' matches directories only; a pattern containing '/' matches
// the slash path relative to the bundled directory, otherwise the base
// name. A missing file yields no patterns.
func LoadIgnoreFile(p string) ([]string, error) {
	f, err := os.Open(p) //nolint:gosec // fixed file name inside the operator's playbook tree
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var patterns []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		patterns = append(patterns, line)
	}
	return patterns, scanner.Err()
}

func matchesIgnore(patterns []string, rel string, isDir bool) bool {
	base := path.Base(rel)
	for _, p := range patterns {
		dirOnly := strings.HasSuffix(p, "/")
		p = strings.TrimSuffix(p, "/")
		if dirOnly && !isDir {
			continue
		}
		subject := base
		if strings.Contains(p, "/") {
			p = strings.TrimPrefix(p, "/")
			subject = rel
		}
		if ok, _ := path.Match(p, subject); ok {
			return true
		}
	}
	return false
}

// IsSafeEntryName reports whether name is a relative, slash-separated path
// with no '..' segments, backslashes, or drive/absolute prefix.
func IsSafeEntryName(name string) bool {
	if name == "" || strings.Contains(name, `\`) || strings.HasPrefix(name, "/") {
		return false
	}
	if len(name) >= 2 && name[1] == ':' {
		return false
	}
	clean := path.Clean(strings.TrimSuffix(name, "/"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	for _, seg := range strings.Split(name, "/") {
		if seg == ".." {
			return false
		}
	}
	return true
}

// Extract unpacks a tar.gz stream from r into dest, which must already
// exist. Every entry is validated before anything is written for it, both
// lexically and against the real (symlink-resolved) filesystem, so a
// symlink created by an earlier entry can't redirect a later one outside
// dest; symlinks must also resolve inside dest.
func Extract(r io.Reader, dest string) error {
	destAbs, err := filepath.Abs(dest)
	if err != nil {
		return err
	}
	root, err := filepath.EvalSymlinks(destAbs)
	if err != nil {
		return err
	}
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer func() { _ = gz.Close() }()
	tr := tar.NewReader(gz)
	var written int64
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !IsSafeEntryName(hdr.Name) {
			return fmt.Errorf("unsafe bundle entry %q", hdr.Name)
		}
		clean := path.Clean(strings.TrimSuffix(hdr.Name, "/"))
		target := filepath.Clean(filepath.Join(root, filepath.FromSlash(clean)))
		if !strings.HasPrefix(target, root+string(filepath.Separator)) {
			return fmt.Errorf("bundle entry %q escapes the destination", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := mkdirWithin(root, target); err != nil {
				return fmt.Errorf("bundle dir %q: %w", hdr.Name, err)
			}
		case tar.TypeReg:
			if hdr.Size < 0 || written+hdr.Size > MaxExtractBytes {
				return fmt.Errorf("bundle exceeds %d bytes uncompressed", MaxExtractBytes)
			}
			if err := mkdirWithin(root, filepath.Dir(target)); err != nil {
				return fmt.Errorf("bundle file %q: %w", hdr.Name, err)
			}
			if err := checkResolvedWithin(root, target); err != nil {
				return fmt.Errorf("bundle file %q: %w", hdr.Name, err)
			}
			mode := os.FileMode(0o600)
			if hdr.Mode&0o111 != 0 {
				mode = 0o700
			}
			n, err := writeFile(target, tr, hdr.Size, mode)
			written += n
			if err != nil {
				return err
			}
		case tar.TypeSymlink:
			if !safeLink(clean, hdr.Linkname) {
				return fmt.Errorf("bundle symlink %q -> %q escapes the bundle", hdr.Name, hdr.Linkname)
			}
			if err := mkdirWithin(root, filepath.Dir(target)); err != nil {
				return fmt.Errorf("bundle symlink %q: %w", hdr.Name, err)
			}
			linkDir, err := filepath.EvalSymlinks(filepath.Dir(target))
			if err != nil {
				return err
			}
			if !isRel(hdr.Name, root, root) || !isRel(hdr.Linkname, linkDir, root) {
				return fmt.Errorf("bundle symlink %q -> %q escapes the destination", hdr.Name, hdr.Linkname)
			}
			if err := os.Symlink(filepath.FromSlash(hdr.Linkname), target); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported bundle entry type %q for %q", hdr.Typeflag, hdr.Name)
		}
	}
}

// mkdirWithin creates dir one level at a time, checking after each step
// that the real (symlink-resolved) path is still inside root.
func mkdirWithin(root, dir string) error {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return err
	}
	if rel == "." {
		return nil
	}
	current := root
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		current = filepath.Join(current, part)
		if err := os.Mkdir(current, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
		resolved, err := filepath.EvalSymlinks(current)
		if err != nil {
			return err
		}
		if !withinRoot(root, resolved) {
			return errors.New("path escapes the destination through a symlink")
		}
	}
	return nil
}

// checkResolvedWithin verifies target's real parent dir is inside root.
func checkResolvedWithin(root, target string) error {
	parent, err := filepath.EvalSymlinks(filepath.Dir(target))
	if err != nil {
		return err
	}
	if !withinRoot(root, filepath.Join(parent, filepath.Base(target))) {
		return errors.New("path escapes the destination through a symlink")
	}
	return nil
}

// isRel resolves candidate from base (a real directory) one segment at a
// time, following existing symlinks, and reports whether every step stays
// inside root. '..' is only allowed before the first name: a name that
// doesn't exist yet could become a symlink later in the archive, and '..'
// after it would then climb from wherever that link points.
func isRel(candidate, base, root string) bool {
	if candidate == "" || filepath.IsAbs(candidate) || strings.HasPrefix(candidate, "/") {
		return false
	}
	current := base
	leading := true
	for _, segment := range strings.Split(filepath.ToSlash(candidate), "/") {
		switch segment {
		case "", ".":
			continue
		case "..":
			if !leading {
				return false
			}
			current = filepath.Dir(current)
		default:
			leading = false
			current = filepath.Join(current, segment)
			resolved, err := filepath.EvalSymlinks(current)
			switch {
			case err == nil:
				current = resolved
			case !errors.Is(err, fs.ErrNotExist):
				return false
			}
		}
		rel, err := filepath.Rel(root, current)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
			return false
		}
	}
	return true
}

func withinRoot(root, p string) bool {
	rel, err := filepath.Rel(root, filepath.Clean(p))
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func writeFile(target string, r io.Reader, size int64, mode os.FileMode) (int64, error) {
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode) //nolint:gosec // target validated by IsSafeEntryName
	if err != nil {
		return 0, err
	}
	n, copyErr := io.CopyN(f, r, size)
	closeErr := f.Close()
	if copyErr != nil {
		return n, copyErr
	}
	return n, closeErr
}

func safeLink(entry, link string) bool {
	if link == "" || strings.Contains(link, `\`) || strings.HasPrefix(link, "/") {
		return false
	}
	if len(link) >= 2 && link[1] == ':' {
		return false
	}
	resolved := path.Clean(path.Join(path.Dir(entry), link))
	return resolved != ".." && !strings.HasPrefix(resolved, "../")
}
