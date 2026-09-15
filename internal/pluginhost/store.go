package pluginhost

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

// Manifest records one installed plugin version without requiring the plugin
// process to be launched again for list and info commands.
type Manifest struct {
	Organization    string    `json:"organization"`
	Name            string    `json:"name"`
	Version         string    `json:"version"`
	Source          string    `json:"source"`
	Checksum        string    `json:"checksum"`
	InstalledAt     time.Time `json:"installed_at"`
	HandlerNames    []string  `json:"handler_names"`
	ProtocolVersion int       `json:"protocol_version"`
	License         string    `json:"license,omitempty"`
	DocSummary      string    `json:"doc_summary,omitempty"`
}

// Namespace returns the playbook identity for the plugin.
func (m Manifest) Namespace() string { return m.Organization + "." + m.Name }

// Store owns ironstate's versioned user plugin cache.
type Store struct {
	root string
}

// NewStore creates a store rooted at root. It is primarily useful to tests.
func NewStore(root string) *Store { return &Store{root: root} }

// DefaultStore returns the per-user plugin store.
func DefaultStore() (*Store, error) {
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, fmt.Errorf("find user cache directory: %w", err)
	}
	return NewStore(filepath.Join(cacheDir, "ironstate", "plugins")), nil
}

// Root returns the store root.
func (s *Store) Root() string { return s.root }

// StorePlugin copies binary into the versioned store and writes its manifest.
func (s *Store) StorePlugin(manifest Manifest, binary string) (Manifest, error) {
	if err := validateIdentity(manifest.Namespace()); err != nil {
		return Manifest{}, err
	}
	if !validPathPart(manifest.Version) {
		return Manifest{}, fmt.Errorf("invalid plugin version %q", manifest.Version)
	}
	if manifest.Source == "" {
		return Manifest{}, fmt.Errorf("plugin source is required")
	}
	if _, err := os.Stat(binary); err != nil {
		return Manifest{}, fmt.Errorf("stat plugin binary: %w", err)
	}

	destinationDir := s.versionDir(manifest.Namespace(), manifest.Version)
	if err := os.MkdirAll(destinationDir, 0o750); err != nil {
		return Manifest{}, fmt.Errorf("create plugin store: %w", err)
	}
	destination := filepath.Join(destinationDir, binaryName(manifest.Name))
	if err := copyFile(binary, destination); err != nil {
		return Manifest{}, err
	}
	checksum, err := checksumFile(destination)
	if err != nil {
		return Manifest{}, err
	}
	manifest.Checksum = checksum
	if manifest.InstalledAt.IsZero() {
		manifest.InstalledAt = time.Now().UTC()
	}
	sort.Strings(manifest.HandlerNames)
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Manifest{}, fmt.Errorf("encode plugin manifest: %w", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "manifest.json"), append(encoded, '\n'), 0o600); err != nil {
		return Manifest{}, fmt.Errorf("write plugin manifest: %w", err)
	}
	return manifest, nil
}

// ListInstalled returns every stored plugin version in deterministic order.
func (s *Store) ListInstalled() ([]Manifest, error) {
	organizations, err := os.ReadDir(s.root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plugin store: %w", err)
	}
	var manifests []Manifest
	for _, organization := range organizations {
		if !organization.IsDir() || !validPathPart(organization.Name()) {
			continue
		}
		names, err := os.ReadDir(filepath.Join(s.root, organization.Name()))
		if err != nil {
			return nil, fmt.Errorf("read plugin organization %q: %w", organization.Name(), err)
		}
		for _, name := range names {
			if !name.IsDir() || !validPathPart(name.Name()) {
				continue
			}
			versions, err := os.ReadDir(filepath.Join(s.root, organization.Name(), name.Name()))
			if err != nil {
				return nil, fmt.Errorf("read plugin %s.%s: %w", organization.Name(), name.Name(), err)
			}
			for _, version := range versions {
				if !version.IsDir() || !validPathPart(version.Name()) {
					continue
				}
				manifest, err := s.GetManifest(organization.Name()+"."+name.Name(), version.Name())
				if err != nil {
					// An orphaned version directory (e.g. left behind by a
					// partial uninstall whose binary was locked) shouldn't take
					// down the whole listing.
					continue
				}
				manifests = append(manifests, manifest)
			}
		}
	}
	sort.Slice(manifests, func(i, j int) bool {
		if manifests[i].Namespace() == manifests[j].Namespace() {
			return manifests[i].Version < manifests[j].Version
		}
		return manifests[i].Namespace() < manifests[j].Namespace()
	})
	return manifests, nil
}

// GetManifest reads one installed plugin manifest.
func (s *Store) GetManifest(namespace, version string) (Manifest, error) {
	if err := validateIdentity(namespace); err != nil || !validPathPart(version) {
		return Manifest{}, fmt.Errorf("invalid plugin reference %s@%s", namespace, version)
	}
	contents, err := os.ReadFile(filepath.Join(s.versionDir(namespace, version), "manifest.json")) //nolint:gosec // namespace and version pass validateIdentity and validPathPart
	if err != nil {
		return Manifest{}, fmt.Errorf("read plugin manifest %s@%s: %w", namespace, version, err)
	}
	var manifest Manifest
	if err := json.Unmarshal(contents, &manifest); err != nil {
		return Manifest{}, fmt.Errorf("decode plugin manifest %s@%s: %w", namespace, version, err)
	}
	if manifest.Namespace() != namespace || manifest.Version != version {
		return Manifest{}, fmt.Errorf("plugin manifest identity does not match %s@%s", namespace, version)
	}
	return manifest, nil
}

// BinaryPath returns the executable path for an installed plugin version.
func (s *Store) BinaryPath(namespace, version string) (string, error) {
	manifest, err := s.GetManifest(namespace, version)
	if err != nil {
		return "", err
	}
	path := filepath.Join(s.versionDir(namespace, version), binaryName(manifest.Name))
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("plugin binary %s@%s: %w", namespace, version, err)
	}
	return path, nil
}

// ResolveVersion resolves latest to the greatest installed semantic version,
// or confirms an exact installed version.
func (s *Store) ResolveVersion(namespace, requested string) (Manifest, error) {
	if requested != "latest" {
		return s.GetManifest(namespace, requested)
	}
	all, err := s.ListInstalled()
	if err != nil {
		return Manifest{}, err
	}
	var matches []Manifest
	for _, manifest := range all {
		if manifest.Namespace() == namespace {
			matches = append(matches, manifest)
		}
	}
	if len(matches) == 0 {
		return Manifest{}, fmt.Errorf("plugin %s is not installed", namespace)
	}
	sort.Slice(matches, func(i, j int) bool { return semver.Compare(matches[i].Version, matches[j].Version) > 0 })
	return matches[0], nil
}

// RemoveVersion removes one installed plugin version. The binary is removed
// before manifest.json so that if it's locked (e.g. a still-running plugin
// process on Windows), the manifest survives - keeping 'plugin list'/'info'
// working and letting the uninstall be retried, instead of leaving behind an
// orphaned directory with no manifest.
func (s *Store) RemoveVersion(namespace, version string) error {
	if err := validateIdentity(namespace); err != nil || !validPathPart(version) {
		return fmt.Errorf("invalid plugin reference %s@%s", namespace, version)
	}
	parts := strings.Split(namespace, ".")
	dir := s.versionDir(namespace, version)
	if err := os.Remove(filepath.Join(dir, binaryName(parts[1]))); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plugin %s@%s: %w", namespace, version, err)
	}
	if err := os.Remove(filepath.Join(dir, "manifest.json")); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove plugin %s@%s: %w", namespace, version, err)
	}
	_ = os.Remove(dir) // best-effort: only succeeds once the directory is empty
	return nil
}

func (s *Store) versionDir(namespace, version string) string {
	parts := strings.Split(namespace, ".")
	return filepath.Join(s.root, parts[0], parts[1], version)
}

func validateIdentity(namespace string) error {
	parts := strings.Split(namespace, ".")
	if len(parts) != 2 || !validPathPart(parts[0]) || !validPathPart(parts[1]) {
		return fmt.Errorf("plugin namespace %q must use organization.plugin form", namespace)
	}
	return nil
}

func validPathPart(value string) bool {
	return value != "" && value != "." && value != ".." && filepath.Base(value) == value && !strings.ContainsAny(value, `\\/`)
}

func binaryName(name string) string {
	result := "ironstate-handler-" + name
	if runtime.GOOS == "windows" {
		return result + ".exe"
	}
	return result
}

func copyFile(source, destination string) error {
	input, err := os.Open(source) //nolint:gosec // source is the Go-installed binary selected by validated plugin identity
	if err != nil {
		return fmt.Errorf("open plugin binary: %w", err)
	}
	defer func() { _ = input.Close() }()
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755) //nolint:gosec // plugin binary must retain executable permission bits
	if err != nil {
		return fmt.Errorf("create stored plugin binary: %w", err)
	}
	_, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return fmt.Errorf("copy plugin binary: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close stored plugin binary: %w", closeErr)
	}
	return nil
}

func checksumFile(path string) (string, error) {
	file, err := os.Open(path) //nolint:gosec // path is built from validated store namespace and version
	if err != nil {
		return "", err
	}
	defer func() { _ = file.Close() }()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(hash.Sum(nil)), nil
}
