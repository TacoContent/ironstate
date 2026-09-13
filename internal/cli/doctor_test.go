package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/pluginhost"
)

func TestQualifiedModulesFindsNestedQualifiedHandlers(t *testing.T) {
	modules := qualifiedModules([]any{map[string]any{
		"actions": []any{map[string]any{"acme.hosts.ensure_entry": map[string]any{}}},
	}})
	if len(modules) != 1 || modules[0] != "acme.hosts.ensure_entry" {
		t.Fatalf("qualifiedModules = %#v", modules)
	}
}

func TestDeclaredPlugin(t *testing.T) {
	plugins := []model.Plugin{{Namespace: "acme.hosts", Version: "v1.0.0"}}
	if !declaredPlugin("acme.hosts", plugins) || declaredPlugin("other.hosts", plugins) {
		t.Fatal("declaredPlugin did not distinguish declared namespace")
	}
}

func TestDoctorResolveInstalledPluginsReportsMissingInstall(t *testing.T) {
	_, problems := doctorResolveInstalledPlugins(pluginhost.NewStore(t.TempDir()), nil, []model.Plugin{{Namespace: "acme.hosts", Version: "v1.0.0"}}, func(*exec.Cmd) (*pluginhost.Client, error) {
		t.Fatal("launch should not run for a missing plugin")
		return nil, nil
	})
	if len(problems) != 1 || !strings.Contains(problems[0], "ironstate plugin install acme.hosts@v1.0.0") {
		t.Fatalf("problems = %#v, want install guidance", problems)
	}
}

func TestDoctorPluginVersionPrefersLockfile(t *testing.T) {
	plugin := model.Plugin{Namespace: "acme.hosts", Version: "v1.0.0"}
	lock := &pluginhost.LockFile{Plugins: map[string]pluginhost.LockedPlugin{
		"acme.hosts": {Version: "v2.0.0", Checksum: "sha256:expected"},
	}}
	version, locked, isLocked := doctorPluginVersion(plugin, lock)
	if !isLocked || version != "v2.0.0" || locked.Checksum != "sha256:expected" {
		t.Fatalf("doctorPluginVersion = %q, %#v, %t", version, locked, isLocked)
	}
}

func TestDoctorResolveInstalledPluginsReportsChecksumUpdate(t *testing.T) {
	store := pluginhost.NewStore(t.TempDir())
	binary := filepath.Join(t.TempDir(), "ironstate-handler-hosts")
	if err := os.WriteFile(binary, []byte("not a real plugin"), 0o755); err != nil { //nolint:gosec // fixture represents an executable plugin binary
		t.Fatal(err)
	}
	manifest, err := store.StorePlugin(pluginhost.Manifest{
		Organization: "acme", Name: "hosts", Version: "v1.0.0", Source: "github.com/acme/ironstate-handler-hosts",
	}, binary)
	if err != nil {
		t.Fatal(err)
	}
	lock := &pluginhost.LockFile{Plugins: map[string]pluginhost.LockedPlugin{
		"acme.hosts": {Version: manifest.Version, Checksum: "sha256:wrong"},
	}}
	_, problems := doctorResolveInstalledPlugins(store, lock, []model.Plugin{{Namespace: "acme.hosts", Version: "latest"}}, func(*exec.Cmd) (*pluginhost.Client, error) {
		return nil, fmt.Errorf("launch should not run after a checksum mismatch")
	})
	if len(problems) != 1 || !strings.Contains(problems[0], "ironstate plugin update acme.hosts") {
		t.Fatalf("problems = %#v, want checksum update guidance", problems)
	}
}
