package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TacoContent/ironstate/internal/scan"
)

func TestRunInitCreatesScaffold(t *testing.T) {
	dir := t.TempDir()
	playbook := filepath.Join(dir, "myplaybook")

	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{playbook}); err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	for _, dirName := range []string{"roles", "tasks", "packages", "hosts", "variables"} {
		info, err := os.Stat(filepath.Join(playbook, dirName))
		if err != nil || !info.IsDir() {
			t.Fatalf("expected directory %s to exist: %v", dirName, err)
		}
	}

	siteYML, err := os.ReadFile(filepath.Join(playbook, "main.yml")) //nolint:gosec // test-only path under t.TempDir()
	if err != nil {
		t.Fatalf("main.yml not created: %v", err)
	}
	if string(siteYML) != minimalDocument {
		t.Fatalf("main.yml content = %q, want %q", siteYML, minimalDocument)
	}

	hostsDir := filepath.Join(playbook, "hosts")
	entries, err := os.ReadDir(hostsDir)
	if err != nil || len(entries) != 2 {
		t.Fatalf("expected exactly two host overlay files, got %v (err=%v)", entries, err)
	}
	for _, entry := range entries {
		if entry.Name() != strings.ToLower(entry.Name()) {
			t.Fatalf("host overlay file name %q is not lowercase", entry.Name())
		}
	}
}

func TestRunInitSkipsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	playbook := filepath.Join(dir, "myplaybook")
	if err := os.MkdirAll(playbook, 0o750); err != nil {
		t.Fatal(err)
	}
	sitePath := filepath.Join(playbook, "main.yml")
	custom := "---\nvars: { custom: true }\ntasks: []\n"
	if err := os.WriteFile(sitePath, []byte(custom), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.RunE(cmd, []string{playbook}); err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	data, err := os.ReadFile(sitePath) //nolint:gosec // test-only path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != custom {
		t.Fatalf("existing main.yml was overwritten: got %q", data)
	}
}

func TestRunInitWithScanPopulatesPlaybook(t *testing.T) {
	dir := t.TempDir()
	playbook := filepath.Join(dir, "myplaybook")

	originalGatherScanItems := gatherScanItems
	gatherScanItems = func(progress func(name string, index, total int)) ([]scan.Item, error) {
		if progress != nil {
			progress("users", 1, 2)
			progress("groups", 2, 2)
		}
		return []scan.Item{
			{Module: "user", Name: "alice", Config: map[string]any{"name": "alice", "state": "present"}},
			{Module: "group", Name: "devs", Config: map[string]any{"name": "devs", "state": "present"}},
		}, nil
	}
	defer func() {
		gatherScanItems = originalGatherScanItems
	}()

	cmd := newInitCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	if err := cmd.Flags().Set("scan", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, []string{playbook}); err != nil {
		t.Fatalf("runInit error: %v", err)
	}

	for _, rel := range []string{
		"main.yml",
		"roles/system/users/main.yml",
		"roles/system/groups/main.yml",
		"roles/system/services/main.yml",
		"roles/packages/main.yml",
	} {
		if _, err := os.Stat(filepath.Join(playbook, rel)); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}

	data, err := os.ReadFile(filepath.Join(playbook, "roles", "system", "users", "main.yml")) //nolint:gosec // test-only path under t.TempDir()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "alice") {
		t.Fatalf("expected scanned user to be written, got %q", data)
	}
}
