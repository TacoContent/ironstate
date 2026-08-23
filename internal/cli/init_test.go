package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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

	siteYML, err := os.ReadFile(filepath.Join(playbook, "site.yml")) //nolint:gosec // test-only path under t.TempDir()
	if err != nil {
		t.Fatalf("site.yml not created: %v", err)
	}
	if string(siteYML) != minimalDocument {
		t.Fatalf("site.yml content = %q, want %q", siteYML, minimalDocument)
	}

	hostsDir := filepath.Join(playbook, "hosts")
	entries, err := os.ReadDir(hostsDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one host overlay file, got %v (err=%v)", entries, err)
	}
	if entries[0].Name() != strings.ToLower(entries[0].Name()) {
		t.Fatalf("host overlay file name %q is not lowercase", entries[0].Name())
	}
}

func TestRunInitSkipsExistingFiles(t *testing.T) {
	dir := t.TempDir()
	playbook := filepath.Join(dir, "myplaybook")
	if err := os.MkdirAll(playbook, 0o750); err != nil {
		t.Fatal(err)
	}
	sitePath := filepath.Join(playbook, "site.yml")
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
		t.Fatalf("existing site.yml was overwritten: got %q", data)
	}
}
