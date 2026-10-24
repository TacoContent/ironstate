package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestEnvironmentAliasesAndSourcesAcrossFileGroupAndLoop(t *testing.T) {
	const key = "IRONSTATE_ALIAS_INTEGRATION_TEST"
	t.Setenv(key, "original")
	root := t.TempDir()
	role := filepath.Join(root, "roles", "example")
	nested := filepath.Join(root, "roles", "nested")
	for _, dir := range []string{role, nested} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "source.txt"), []byte("from-file"), 0o600); err != nil {
		t.Fatal(err)
	}
	shellType := "sh"
	if runtime.GOOS == "windows" {
		shellType = "cmd"
	}
	files := map[string]string{
		filepath.Join(root, "main.yml"): fmt.Sprintf(`
environment:
  IRONSTATE_ALIAS_INTEGRATION_TEST:
    valueFrom:
      file: source.txt
  IRONSTATE_SHELL_SOURCE_TEST:
    valueFrom:
      task:
        shell: {type: %s, command: "echo from-shell"}
        value: "${{ stdout | trim }}"
tasks:
  - name: root sources
    assert:
      that:
        - envs.IRONSTATE_ALIAS_INTEGRATION_TEST == 'from-file'
        - envs.IRONSTATE_SHELL_SOURCE_TEST == 'from-shell'
  - include: {name: roles/example}
  - name: unscoped loop with scoped children
    items: [alpha, beta]
    actions:
      - environment:
          IRONSTATE_ALIAS_INTEGRATION_TEST: "${{ item }}"
        assert:
          that:
            - "'${{ '' | env('IRONSTATE_ALIAS_INTEGRATION_TEST') }}' == '${{ item }}'"
  - name: scope restored
    assert:
      that:
        - envs.IRONSTATE_ALIAS_INTEGRATION_TEST == 'from-file'
`, shellType),
		filepath.Join(role, "main.yml"): `
environment:
  IRONSTATE_ALIAS_INTEGRATION_TEST: "package:${{ envs.IRONSTATE_ALIAS_INTEGRATION_TEST }}"
tasks:
  - name: group
    environment:
      IRONSTATE_ALIAS_INTEGRATION_TEST: "group:${{ envs.IRONSTATE_ALIAS_INTEGRATION_TEST }}"
    actions:
      - name: looped leaf
        items: [one, two]
        envs:
          IRONSTATE_ALIAS_INTEGRATION_TEST: "${{ item }}:${{ envs.IRONSTATE_ALIAS_INTEGRATION_TEST }}"
        assert:
          fail_msg: "actual=${{ envs.IRONSTATE_ALIAS_INTEGRATION_TEST }}, item=${{ item }}"
          that:
            - "envs.IRONSTATE_ALIAS_INTEGRATION_TEST == '${{ item }}:group:package:from-file'"
      - include: {name: roles/nested}
`,
		filepath.Join(nested, "main.yml"): `
tasks:
  - name: inherited env filter stays deferred through nested include
    assert:
      that:
        - "'${{ '' | env('IRONSTATE_ALIAS_INTEGRATION_TEST') }}' == 'group:package:from-file'"
`,
	}
	for path, content := range files {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	cmd, err := newRootCommand()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	cmd.SetArgs([]string{"--playbook", filepath.Join(root, "main.yml"), "--output", "json"})
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v\n%s", err, output.String())
	}
	if got := os.Getenv(key); got != "original" {
		t.Fatalf("environment leaked: %q", got)
	}
	if !strings.Contains(output.String(), "scope restored") {
		t.Fatalf("final assertion did not run: %s", output.String())
	}
}
