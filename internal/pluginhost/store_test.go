package pluginhost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStorePluginResolvesListsAndRemovesVersion(t *testing.T) {
	store := NewStore(t.TempDir())
	source := filepath.Join(t.TempDir(), binaryName("hosts"))
	if err := os.WriteFile(source, []byte("plugin"), 0o755); err != nil { //nolint:gosec // fixture represents an executable plugin binary
		t.Fatal(err)
	}
	manifest, err := store.StorePlugin(Manifest{
		Organization: "acme", Name: "hosts", Version: "v1.2.3", Source: "github.com/acme/ironstate-handler-hosts", HandlerNames: []string{"ensure_entry"},
	}, source)
	if err != nil {
		t.Fatalf("StorePlugin: %v", err)
	}
	if manifest.Checksum == "" {
		t.Fatal("StorePlugin did not calculate a checksum")
	}
	resolved, err := store.ResolveVersion("acme.hosts", "latest")
	if err != nil || resolved.Version != "v1.2.3" {
		t.Fatalf("ResolveVersion = %#v, %v", resolved, err)
	}
	if _, err := store.BinaryPath("acme.hosts", "v1.2.3"); err != nil {
		t.Fatalf("BinaryPath: %v", err)
	}
	installed, err := store.ListInstalled()
	if err != nil || len(installed) != 1 {
		t.Fatalf("ListInstalled = %#v, %v", installed, err)
	}
	if err := store.RemoveVersion("acme.hosts", "v1.2.3"); err != nil {
		t.Fatalf("RemoveVersion: %v", err)
	}
	if _, err := store.GetManifest("acme.hosts", "v1.2.3"); err == nil {
		t.Fatal("GetManifest succeeded after RemoveVersion")
	}
}

func TestResolveVersionUsesSemanticVersionOrder(t *testing.T) {
	store := NewStore(t.TempDir())
	source := filepath.Join(t.TempDir(), binaryName("hosts"))
	if err := os.WriteFile(source, []byte("plugin"), 0o755); err != nil { //nolint:gosec // fixture represents an executable plugin binary
		t.Fatal(err)
	}
	for _, version := range []string{"v1.9.0", "v1.10.0"} {
		if _, err := store.StorePlugin(Manifest{Organization: "acme", Name: "hosts", Version: version, Source: "github.com/acme/ironstate-handler-hosts"}, source); err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := store.ResolveVersion("acme.hosts", "latest")
	if err != nil || manifest.Version != "v1.10.0" {
		t.Fatalf("ResolveVersion = %#v, %v", manifest, err)
	}
}

func TestLockFilePinsAndVerifiesPlugin(t *testing.T) {
	dir := t.TempDir()
	manifest := Manifest{Organization: "acme", Name: "hosts", Version: "v1.0.0", Checksum: "sha256:abc"}
	lock := LockFile{Plugins: map[string]LockedPlugin{"acme.hosts": {Version: "v1.0.0", Checksum: "sha256:abc"}}}
	if err := WriteLockFile(dir, lock); err != nil {
		t.Fatalf("WriteLockFile: %v", err)
	}
	loaded, err := LoadLockFile(dir)
	if err != nil || loaded.Plugins["acme.hosts"] != lock.Plugins["acme.hosts"] {
		t.Fatalf("LoadLockFile = %#v, %v", loaded, err)
	}
	if err := VerifyChecksum(manifest, loaded.Plugins["acme.hosts"]); err != nil {
		t.Fatalf("VerifyChecksum: %v", err)
	}
	loaded.Plugins["acme.hosts"] = LockedPlugin{Version: "v1.0.0", Checksum: "sha256:changed"}
	if err := VerifyChecksum(manifest, loaded.Plugins["acme.hosts"]); err == nil {
		t.Fatal("VerifyChecksum accepted mismatched checksum")
	}
}
