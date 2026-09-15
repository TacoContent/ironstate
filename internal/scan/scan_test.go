package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
)

type stubScanner struct {
	name  string
	role  string
	items []Item
}

func (s stubScanner) Name() string { return s.name }

func (s stubScanner) Role() string { return s.role }

func (s stubScanner) Scan() ([]Item, error) { return s.items, nil }

type stubScanHandler struct{}

func (stubScanHandler) Test(map[string]any, string, engine.Context) (bool, error) { return false, nil }
func (stubScanHandler) Describe(map[string]any, engine.Action, engine.Context) (string, error) {
	return "", nil
}
func (stubScanHandler) Install(map[string]any, string, engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}
func (stubScanHandler) Uninstall(map[string]any, string, engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, nil
}
func (stubScanHandler) ScanRole() string { return "roles/system/hosts" }
func (stubScanHandler) Scan(engine.Context) ([]engine.ScanItem, error) {
	return []engine.ScanItem{{Module: "entry", Name: "build.local"}}, nil
}

func TestRegistryScanAllWithProgress(t *testing.T) {
	reg := &Registry{}
	reg.Register(stubScanner{name: "users", role: "roles/system/users", items: []Item{{Name: "alice"}}})
	reg.Register(stubScanner{name: "groups", role: "roles/system/groups", items: []Item{{Name: "devs"}}})

	var seen []string
	items, err := reg.ScanAllWithProgress(func(name string, index, total int) {
		seen = append(seen, name)
		if total != 2 {
			t.Fatalf("total = %d, want 2", total)
		}
		if index < 1 || index > total {
			t.Fatalf("index = %d, total = %d", index, total)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if len(seen) != 2 || seen[0] != "users" || seen[1] != "groups" {
		t.Fatalf("seen = %v, want [users groups]", seen)
	}
	if items[0].Role != "roles/system/users" || items[1].Role != "roles/system/groups" {
		t.Fatalf("roles = %q, %q, want stamped from each scanner", items[0].Role, items[1].Role)
	}
}

func TestBuildTaskListUsesLogForEmptyScan(t *testing.T) {
	tasks := buildTaskList(nil, "system/groups")
	if len(tasks) != 1 {
		t.Fatalf("tasks = %d, want 1", len(tasks))
	}
	if got := tasks[0]["name"]; got != "No items discovered for this baseline" {
		t.Fatalf("name = %v, want %q", got, "No items discovered for this baseline")
	}
	logTask, ok := tasks[0]["log"].(map[string]any)
	if !ok {
		t.Fatalf("log task missing or wrong type: %#v", tasks[0]["log"])
	}
	if got := logTask["message"]; got != "no matching items found" {
		t.Fatalf("log.message = %v, want %q", got, "no matching items found")
	}
	if _, exists := tasks[0]["debug"]; exists {
		t.Fatalf("debug task should not be present: %#v", tasks[0])
	}
}

func TestGeneratePlaybookIncludesPluginRole(t *testing.T) {
	target := t.TempDir()
	err := GeneratePlaybook(target, []Item{{
		Module: "camalot.hosts.entry",
		Name:   "build.local",
		Role:   "roles/system/hosts",
		Config: map[string]any{"ip": "10.0.0.12", "hostname": "build.local"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	rolePath := filepath.Join(target, "roles", "system", "hosts", "main.yml")
	contents, err := os.ReadFile(rolePath) //nolint:gosec // t.TempDir-derived test file
	if err != nil {
		t.Fatalf("read plugin role: %v", err)
	}
	if !strings.Contains(string(contents), "camalot.hosts.entry") {
		t.Fatalf("plugin role = %q, want qualified module", contents)
	}
	main, err := os.ReadFile(filepath.Join(target, "main.yml")) //nolint:gosec // t.TempDir-derived test file
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(main), "roles/system/hosts") {
		t.Fatalf("main playbook = %q, want plugin role include", main)
	}
}

func TestPluginScannerQualifiesUnqualifiedItemModule(t *testing.T) {
	registry := &Registry{}
	for _, scanner := range scannersForHandlers(map[string]engine.Handler{"camalot.hosts.entry": stubScanHandler{}}) {
		registry.Register(scanner)
	}
	items, err := registry.ScanAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Module != "camalot.hosts.entry" {
		t.Fatalf("scanned items = %#v, want qualified plugin module", items)
	}
}

func TestDefaultRegistrySkipsNamespacedBuiltinAliases(t *testing.T) {
	names := NewRegistry().ListNames()
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if strings.HasPrefix(name, "ironstate.builtin.") {
			t.Fatalf("builtin alias %q should not have its own scanner", name)
		}
		if seen[name] {
			t.Fatalf("scanner %q was registered more than once", name)
		}
		seen[name] = true
	}
}
