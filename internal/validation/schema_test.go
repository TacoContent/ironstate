package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateFileReportsSourceLocation(t *testing.T) {
	originalLoader := schemaLoader
	schemaLoader = func(string) ([]byte, string, error) {
		path := filepath.Join("..", "..", "ironstate.schema.json")
		data, err := os.ReadFile(path)
		return data, path, err
	}
	defer func() { schemaLoader = originalLoader }()

	playbook := filepath.Join(t.TempDir(), "main.yml")
	content := "tasks:\n  - name: invalid winget\n    winget:\n      package: 42\n"
	if err := os.WriteFile(playbook, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	err := ValidateFile(playbook)
	if err == nil {
		t.Fatal("ValidateFile succeeded for an invalid playbook")
	}
	message := err.Error()
	for _, expected := range []string{playbook, ":4:", "/tasks/0/winget/package"} {
		if !strings.Contains(message, expected) {
			t.Fatalf("validation error %q does not contain %q", message, expected)
		}
	}
}

func TestValidatePlaybookRecursesThroughYAMLTree(t *testing.T) {
	originalLoader := schemaLoader
	schemaLoader = func(string) ([]byte, string, error) {
		path := filepath.Join("..", "..", "ironstate.schema.json")
		data, err := os.ReadFile(path)
		return data, path, err
	}
	defer func() { schemaLoader = originalLoader }()

	root := t.TempDir()
	rootFile := filepath.Join(root, "main.yml")
	nestedFile := filepath.Join(root, "roles", "example", "main.yml")
	if err := os.MkdirAll(filepath.Dir(nestedFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rootFile, []byte("tasks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nestedFile, []byte("tasks:\n  - winget:\n      package: 42\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	valid, err := ValidatePlaybook(rootFile)
	if err == nil || !strings.Contains(err.Error(), nestedFile) {
		t.Fatalf("ValidatePlaybook error = %v, want nested-file validation failure", err)
	}
	if len(valid) != 1 || valid[0] != rootFile {
		t.Fatalf("valid files = %#v, want only %q", valid, rootFile)
	}
}
