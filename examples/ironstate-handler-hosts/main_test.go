package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/sdk/handler"
)

func testItem(path string) map[string]any {
	return map[string]any{"path": path, "ip": "10.0.0.12", "hostname": "build.local"}
}

func TestHostsHandlerInstallTestAndUninstallAreIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	h := hostsHandler{}
	item := testItem(path)

	satisfied, err := h.Test(item, "", handler.Context{})
	if err != nil || satisfied {
		t.Fatalf("initial Test = %v, %v; want false, nil", satisfied, err)
	}
	if _, err := h.Install(item, "", handler.Context{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	if _, err := h.Install(item, "", handler.Context{}); err != nil {
		t.Fatalf("second Install returned error: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read hosts file: %v", err)
	}
	if got := strings.Count(string(contents), "10.0.0.12 build.local"); got != 1 {
		t.Fatalf("mapping count = %d, want 1; contents = %q", got, contents)
	}
	satisfied, err = h.Test(item, "", handler.Context{})
	if err != nil || !satisfied {
		t.Fatalf("installed Test = %v, %v; want true, nil", satisfied, err)
	}
	if _, err := h.Uninstall(item, "", handler.Context{}); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}
	if _, err := h.Uninstall(item, "", handler.Context{}); err != nil {
		t.Fatalf("second Uninstall returned error: %v", err)
	}
	satisfied, err = h.Test(item, "", handler.Context{})
	if err != nil || satisfied {
		t.Fatalf("uninstalled Test = %v, %v; want false, nil", satisfied, err)
	}
}

func TestHostsHandlerPreservesCommentsAndReplacesHostname(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	initial := "# keep this comment\n192.0.2.10 build.local alias.local\n127.0.0.1 localhost\n"
	if err := os.WriteFile(path, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	item := testItem(path)
	item["ip"] = "192.0.2.20"
	if _, err := (hostsHandler{}).Install(item, "", handler.Context{}); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(contents)
	if strings.Contains(text, "192.0.2.10 build.local") || !strings.Contains(text, "192.0.2.20 build.local") || !strings.Contains(text, "# keep this comment") {
		t.Fatalf("unexpected hosts content: %q", text)
	}
}

func TestHostsHandlerFactProducerAndScan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hosts")
	if err := os.WriteFile(path, []byte("10.0.0.12 build.local\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := hostsHandler{}
	name, ok := h.FactName(map[string]any{"name": "$build_host"})
	if name != "build_host" || !ok {
		t.Fatalf("FactName = %q, %v", name, ok)
	}
	items, err := h.Scan(handler.Context{Flat: map[string]any{"hosts_path": path}})
	if err != nil || len(items) != 1 {
		t.Fatalf("Scan = %#v, %v; want one item", items, err)
	}
	if items[0].Module != "ensure_entry" || items[0].Config["hostname"] != "build.local" {
		t.Fatalf("scanned item = %#v", items[0])
	}
}

func TestHostsHandlerRejectsMissingFields(t *testing.T) {
	if _, err := (hostsHandler{}).Test(map[string]any{"hostname": "build.local"}, "", handler.Context{}); err == nil {
		t.Fatal("Test accepted missing ip")
	}
}
