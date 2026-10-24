package validation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateEnvironmentAliasesAndSources(t *testing.T) {
	originalLoader := schemaLoader
	schemaLoader = func(string) ([]byte, string, error) {
		path := filepath.Join("..", "..", "ironstate.schema.json")
		data, err := os.ReadFile(path)
		return data, path, err
	}
	t.Cleanup(func() { schemaLoader = originalLoader })
	for _, tc := range []struct {
		name    string
		content string
		valid   bool
	}{
		{"root-alias", "environment: {A: abc}\nenvs: {B: true}\ntasks: []", true},
		{"file", "envs: {A: {valueFrom: {file: value.txt}}}", true},
		{"task", `environment: {A: {valueFrom: {task: {shell: {command: echo value}, value: "${{ stdout }}"}}}}`, true},
		{"silent-task-source", `environment: {A: {valueFrom: {task: {shell: {command: echo value, silent: true}, value: "${{ stdout }}"}}}}`, true},
		{"silent-shell-and-fact", "tasks:\n  - shell: {command: echo value, silent: true}\n  - fact: {name: value, shell: {command: echo value, silent: '${{ vars.quiet }}'}}", true},
		{"failed-when-shell-and-template", `tasks:
  - failed_when:
      - "${{ rc != 0 }}"
      - shell: {command: "! command -v my_command >/dev/null", type: bash}
        value: "${{ rc == 0 }}"
    shell: |
      echo task
    creates: [/tmp/task-marker]
    removes: [/tmp/my-command]
`, true},
		{"failed-when-shell-missing-value", `tasks:
  - failed_when:
      - shell: {command: echo check}
    shell: {command: echo task}
`, false},
		{"invalid-silent", "tasks:\n  - shell: {command: echo value, silent: invalid}", false},
		{"value-only", "envs: {A: {valueFrom: {task: {value: 42}}}}", true},
		{"group-and-leaf", "tasks:\n  - environment: {A: abc}\n    actions:\n      - envs: {B: {valueFrom: {file: value.txt}}}\n        log: {message: ok}", true},
		{"unknown-source", "envs: {A: {valueFrom: {unknown: x}}}", false},
		{"multiple-sources", "envs: {A: {valueFrom: {file: x, task: {value: x}}}}", false},
		{"missing-value", "envs: {A: {valueFrom: {task: {shell: {command: echo x}}}}}", false},
		{"invalid-shell", "envs: {A: {valueFrom: {task: {shell: {}, value: x}}}}", false},
		{"shell-control", "envs: {A: {valueFrom: {task: {shell: {command: echo x, creates: [x]}, value: x}}}}", false},
		{"unknown-task-field", "envs: {A: {valueFrom: {task: {value: x, extra: x}}}}", false},
		{"unknown-entry-field", "envs: {A: {valueFrom: {file: x}, extra: x}}", false},
		{"map-without-source", "envs: {A: {value: x}}", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "main.yml")
			if err := os.WriteFile(path, []byte(tc.content), 0o600); err != nil {
				t.Fatal(err)
			}
			err := ValidateFile(path)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

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

func TestValidateJinja2TemplateEngines(t *testing.T) {
	originalLoader := schemaLoader
	schemaLoader = func(string) ([]byte, string, error) {
		path := filepath.Join("..", "..", "ironstate.schema.json")
		data, err := os.ReadFile(path)
		return data, path, err
	}
	defer func() { schemaLoader = originalLoader }()

	playbook := filepath.Join(t.TempDir(), "main.yml")
	content := `tasks:
  - template:
      src: template.j2
      dest: output
      engine: jinja2
  - blockinfile:
      dest: output
      template:
        src: template.j2
        engine: jinja2
`
	if err := os.WriteFile(playbook, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateFile(playbook); err != nil {
		t.Fatal(err)
	}
}
