package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/pluginhost"
)

func setupPluginUpdate(t *testing.T, playbook string) (*pluginhost.Store, string, *[]string) {
	t.Helper()
	store := pluginTestStoreFixture(t)
	binary := filepath.Join(t.TempDir(), "ironstate-handler-hosts")
	if err := os.WriteFile(binary, []byte("fixture v2"), 0o755); err != nil { //nolint:gosec // fixture only needs to exist for the store
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.yml"), []byte(playbook), 0o600); err != nil {
		t.Fatal(err)
	}
	installs := &[]string{}
	originalStore, originalInstall := pluginUpdateStore, pluginUpdateInstall
	t.Cleanup(func() { pluginUpdateStore, pluginUpdateInstall = originalStore, originalInstall })
	pluginUpdateStore = func() (*pluginhost.Store, error) { return store, nil }
	pluginUpdateInstall = func(_ context.Context, store *pluginhost.Store, namespace, version string) (pluginhost.Manifest, error) {
		*installs = append(*installs, namespace+"@"+version)
		return store.StorePlugin(pluginhost.Manifest{Organization: "acme", Name: "hosts", Version: "v2.0.0", Source: "github.com/acme/ironstate-handler-hosts"}, binary)
	}
	return store, dir, installs
}

func runPluginUpdate(t *testing.T, args ...string) (string, error) {
	t.Helper()
	command := newPluginUpdateCommand()
	var out bytes.Buffer
	command.SetOut(&out)
	command.SetErr(&out)
	command.SetArgs(args)
	err := command.Execute()
	return out.String(), err
}

func TestPluginUpdateWritesLockFile(t *testing.T) {
	store, dir, installs := setupPluginUpdate(t, "plugins:\n  - use: acme.hosts@v1.0.0\ntasks: []\n")
	output, err := runPluginUpdate(t, "acme.hosts", "--playbook", dir)
	if err != nil {
		t.Fatalf("plugin update: %v\n%s", err, output)
	}
	if strings.Join(*installs, ",") != "acme.hosts@latest" {
		t.Fatalf("installs = %v", *installs)
	}
	lock, err := pluginhost.LoadLockFile(dir)
	if err != nil || lock == nil {
		t.Fatalf("LoadLockFile = %#v, %v", lock, err)
	}
	updated, _ := store.ResolveVersion("acme.hosts", "v2.0.0")
	if got := lock.Plugins["acme.hosts"]; got.Version != "v2.0.0" || got.Checksum != updated.Checksum {
		t.Fatalf("lock entry = %#v, want v2.0.0 %s", got, updated.Checksum)
	}
}

func TestPluginUpdateLockFileOnlyInstallsNothing(t *testing.T) {
	store, dir, installs := setupPluginUpdate(t, "plugins:\n  - use: acme.hosts@v1.0.0\ntasks: []\n")
	output, err := runPluginUpdate(t, "--lock-file-only", "--playbook", dir)
	if err != nil {
		t.Fatalf("plugin update --lock-file-only: %v\n%s", err, output)
	}
	if len(*installs) != 0 {
		t.Fatalf("--lock-file-only installed %v", *installs)
	}
	installed, _ := store.ResolveVersion("acme.hosts", "v1.0.0")
	lock, err := pluginhost.LoadLockFile(dir)
	if err != nil || lock.Plugins["acme.hosts"] != (pluginhost.LockedPlugin{Version: "v1.0.0", Checksum: installed.Checksum}) {
		t.Fatalf("lock = %#v, %v", lock, err)
	}
	if err := pluginhost.VerifyChecksum(installed, lock.Plugins["acme.hosts"]); err != nil {
		t.Fatalf("written lock does not verify: %v", err)
	}
}

func TestPluginUpdateLockFileOnlyRejectsUndeclaredPlugin(t *testing.T) {
	_, dir, _ := setupPluginUpdate(t, "plugins:\n  - use: acme.hosts@v1.0.0\ntasks: []\n")
	if _, err := runPluginUpdate(t, "acme.other", "--lock-file-only", "--playbook", dir); err == nil {
		t.Fatal("--lock-file-only accepted a plugin the playbook does not declare")
	}
}

func TestPluginUpdateLockFileOnlyRequiresInstalledPlugin(t *testing.T) {
	_, dir, _ := setupPluginUpdate(t, "plugins:\n  - use: acme.hosts@v9.0.0\ntasks: []\n")
	_, err := runPluginUpdate(t, "--lock-file-only", "--playbook", dir)
	if err == nil || !strings.Contains(err.Error(), "plugin install acme.hosts@v9.0.0") {
		t.Fatalf("error = %v, want install hint", err)
	}
}
