package pluginhost

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const lockFileName = "ironstate.lock.yaml"

// LockFile pins plugin versions and checksums for one playbook tree.
type LockFile struct {
	Plugins map[string]LockedPlugin `yaml:"plugins"`
}

// LockedPlugin is the reproducible identity of one installed plugin.
type LockedPlugin struct {
	Version  string `yaml:"version"`
	Checksum string `yaml:"checksum"`
}

// LoadLockFile loads the optional lockfile beside the root playbook.
func LoadLockFile(root string) (*LockFile, error) {
	contents, err := os.ReadFile(filepath.Join(root, lockFileName)) //nolint:gosec // root is the resolved playbook directory and filename is fixed
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read plugin lockfile: %w", err)
	}
	var lock LockFile
	if err := yaml.Unmarshal(contents, &lock); err != nil {
		return nil, fmt.Errorf("decode plugin lockfile: %w", err)
	}
	for namespace, plugin := range lock.Plugins {
		if err := validateIdentity(namespace); err != nil || !validPathPart(plugin.Version) || plugin.Checksum == "" {
			return nil, fmt.Errorf("invalid plugin lock entry %q", namespace)
		}
	}
	return &lock, nil
}

// WriteLockFile writes a deterministic lockfile beside the root playbook.
func WriteLockFile(root string, lock LockFile) error {
	if lock.Plugins == nil {
		lock.Plugins = map[string]LockedPlugin{}
	}
	for namespace, plugin := range lock.Plugins {
		if err := validateIdentity(namespace); err != nil || !validPathPart(plugin.Version) || plugin.Checksum == "" {
			return fmt.Errorf("invalid plugin lock entry %q", namespace)
		}
	}
	contents, err := yaml.Marshal(lock)
	if err != nil {
		return fmt.Errorf("encode plugin lockfile: %w", err)
	}
	return os.WriteFile(filepath.Join(root, lockFileName), contents, 0o600)
}

// VerifyChecksum rejects a manifest whose binary no longer matches its lock.
func VerifyChecksum(manifest Manifest, locked LockedPlugin) error {
	if manifest.Version != locked.Version || manifest.Checksum != locked.Checksum {
		return fmt.Errorf("plugin %s checksum does not match ironstate.lock.yaml; run: ironstate plugin update %s", manifest.Namespace(), manifest.Namespace())
	}
	return nil
}
