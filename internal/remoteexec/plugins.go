package remoteexec

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/TacoContent/ironstate/internal/pluginhost"
)

// PluginStore opens the controller's plugin store; overridable for tests.
var PluginStore = pluginhost.DefaultStore

// EnsurePlugins copies the playbook's declared plugins from the controller's
// store into the target's plugin store, each file SHA-256 verified. Call it
// only for a target on the controller's own OS/arch (plugin binaries are
// platform-specific). Plugins the controller hasn't installed are left for
// the target's own store or --allow-plugin-install. Returns the shipped
// plugin identities.
func EnsurePlugins(ctx context.Context, t Transport, info ProbeInfo, job *PreparedJob) ([]string, error) {
	store, err := PluginStore()
	if err != nil {
		return nil, err
	}
	lock, err := pluginhost.LoadLockFile(job.root)
	if err != nil {
		return nil, err
	}
	var shipped []string
	for _, declared := range job.Plugins {
		version := declared.Version
		if lock != nil {
			if locked, ok := lock.Plugins[declared.Namespace]; ok {
				version = locked.Version
			}
		}
		manifest, err := store.ResolveVersion(declared.Namespace, version)
		if err != nil {
			continue
		}
		binary, err := store.BinaryPath(declared.Namespace, manifest.Version)
		if err != nil {
			continue
		}
		manifestFile, err := store.ManifestPath(declared.Namespace, manifest.Version)
		if err != nil {
			continue
		}
		parts := strings.SplitN(declared.Namespace, ".", 2)
		dir := joinRemotePath(info.OS, info.StateDir, "plugins", parts[0], parts[1], manifest.Version)
		identity := declared.Namespace + "@" + manifest.Version
		// Binary first: the target only sees the plugin once its manifest lands.
		for _, file := range []string{binary, manifestFile} {
			sum, err := fileSHA256(file)
			if err != nil {
				return shipped, err
			}
			remotePath := joinRemotePath(info.OS, dir, filepath.Base(file))
			if _, err := ensureRemoteFile(ctx, t, info, LocalAgent{Path: file, SHA256: sum}, remotePath, "plugin "+identity); err != nil {
				return shipped, err
			}
		}
		shipped = append(shipped, identity)
	}
	return shipped, nil
}
