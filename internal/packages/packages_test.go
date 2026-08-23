package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadFileResolvesRelativePaths(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.yml"), `
tasks:
  - name: render
    template: { src: templates/foo.tmpl, dest: ~/foo, engine: gotemplate }
  - name: group
    actions:
      - name: nested copy
        copy: { src: files/bar.txt, dest: ~/bar }
  - name: absolute stays put
    copy: { src: /already/absolute, dest: ~/x }
  - name: url stays put
    shell: { command: echo hi }
`)
	doc, err := LoadFile(filepath.Join(dir, "main.yml"), "")
	if err != nil {
		t.Fatal(err)
	}
	m := doc.(map[string]any)
	tasks := m["tasks"].([]any)

	render := tasks[0].(map[string]any)["template"].(map[string]any)
	wantSrc := filepath.Join(dir, "templates", "foo.tmpl")
	if render["src"] != wantSrc {
		t.Errorf("template.src = %v, want %v", render["src"], wantSrc)
	}

	nested := tasks[1].(map[string]any)["actions"].([]any)[0].(map[string]any)["copy"].(map[string]any)
	wantNestedSrc := filepath.Join(dir, "files", "bar.txt")
	if nested["src"] != wantNestedSrc {
		t.Errorf("nested copy.src = %v, want %v", nested["src"], wantNestedSrc)
	}

	abs := tasks[2].(map[string]any)["copy"].(map[string]any)
	if abs["src"] != "/already/absolute" {
		t.Errorf("absolute src was rewritten: %v", abs["src"])
	}
}

func TestMergeVarsDeepMerge(t *testing.T) {
	base := map[string]any{
		"editor": "code",
		"shell":  map[string]any{"pwsh": map[string]any{"dev": "~/Development"}},
	}
	overlay := map[string]any{
		"editor": "nvim",
		"shell":  map[string]any{"pwsh": map[string]any{"ws": "~/work"}},
	}
	merged := MergeVars(base, overlay)
	if merged["editor"] != "nvim" {
		t.Errorf("editor = %v", merged["editor"])
	}
	pwsh := merged["shell"].(map[string]any)["pwsh"].(map[string]any)
	if pwsh["dev"] != "~/Development" || pwsh["ws"] != "~/work" {
		t.Errorf("deep merge did not preserve both keys: %#v", pwsh)
	}
}

func TestMergeDocumentsAppendsTasksAndDeepMergesVars(t *testing.T) {
	base := map[string]any{
		"vars":  map[string]any{"a": 1.0},
		"tasks": []any{"base-task"},
	}
	overlay := map[string]any{
		"vars":  map[string]any{"b": 2.0},
		"tasks": []any{"overlay-task"},
	}
	merged := MergeDocuments(base, overlay)
	tasks := merged["tasks"].([]any)
	if len(tasks) != 2 || tasks[0] != "base-task" || tasks[1] != "overlay-task" {
		t.Fatalf("tasks = %#v, want base then overlay appended", tasks)
	}
	vars := merged["vars"].(map[string]any)
	if vars["a"] != 1.0 || vars["b"] != 2.0 {
		t.Fatalf("vars = %#v, want both keys deep-merged", vars)
	}
}

func TestLoadHierarchyMergesHostAndUserOverlays(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), `
vars:
  editor: code
tasks:
  - name: base task
    log: { message: base }
`)
	writeFile(t, filepath.Join(dir, "hosts", "TESTHOST.yml"), `
tasks:
  - name: host task
    log: { message: host }
`)
	writeFile(t, filepath.Join(dir, "variables", "testuser.yml"), `
vars:
  editor: nvim
tasks:
  - name: user task
    log: { message: user }
`)

	t.Setenv("COMPUTERNAME", "TESTHOST")
	t.Setenv("USERNAME", "testuser")

	doc, err := LoadHierarchy(filepath.Join(dir, "site.yml"))
	if err != nil {
		t.Fatal(err)
	}
	m := doc.(map[string]any)
	tasks := m["tasks"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("tasks = %#v, want base+host+user appended in order", tasks)
	}
	if m["vars"].(map[string]any)["editor"] != "nvim" {
		t.Fatalf("vars.editor = %v, want user overlay to win", m["vars"].(map[string]any)["editor"])
	}
}

func TestLoadHierarchySkipsMissingOverlays(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), `
tasks:
  - name: base task
    log: { message: base }
`)
	t.Setenv("COMPUTERNAME", "NOSUCHHOST")
	t.Setenv("USERNAME", "nosuchuser")

	doc, err := LoadHierarchy(filepath.Join(dir, "site.yml"))
	if err != nil {
		t.Fatal(err)
	}
	tasks := doc.(map[string]any)["tasks"].([]any)
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v, want only base", tasks)
	}
}

func TestLoadIncludedPackageResolvesTemplateContext(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "cli", "main.yml"), `
vars:
  default_state: present
tasks:
  - name: install ${{ package.name }}
    winget:
      package: ${{ inputs.id }}
      state: ${{ package.state }}
`)

	fset := filters.New()
	includeSpec := map[string]any{
		"name":  "cli",
		"state": "present",
		"with":  map[string]any{"id": "FiloSottile.age"},
	}
	included, err := LoadIncludedPackage(includeSpec, root, map[string]any{}, map[string]any{}, fset)
	if err != nil {
		t.Fatal(err)
	}
	if included == nil {
		t.Fatal("expected a resolved package, got nil")
	}
	data := included.Data.(map[string]any)
	tasks := data["tasks"].([]any)
	leaf := tasks[0].(map[string]any)
	if leaf["name"] != "install cli" {
		t.Errorf("name = %v, want 'install cli' (package.name resolved)", leaf["name"])
	}
	winget := leaf["winget"].(map[string]any)
	if winget["package"] != "FiloSottile.age" {
		t.Errorf("winget.package = %v, want resolved inputs.id", winget["package"])
	}
	if winget["state"] != "present" {
		t.Errorf("winget.state = %v, want resolved package.state", winget["state"])
	}
	if included.Package["name"] != "cli" {
		t.Errorf("Package.name = %v", included.Package["name"])
	}
}

func TestLoadIncludedPackageMissingReturnsNilNotError(t *testing.T) {
	root := t.TempDir()
	included, err := LoadIncludedPackage(map[string]any{"name": "does-not-exist"}, root, nil, nil, filters.New())
	if err != nil {
		t.Fatalf("expected no error for a missing package, got %v", err)
	}
	if included != nil {
		t.Fatalf("expected nil result for a missing package, got %#v", included)
	}
}

func TestImportEnvFileSetsEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".env"), "FOO=bar\n# comment\nQUOTED=\"with spaces\"\n")
	if err := ImportEnvFile(filepath.Join(dir, ".env")); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("FOO") != "bar" {
		t.Errorf("FOO = %q", os.Getenv("FOO"))
	}
	if os.Getenv("QUOTED") != "with spaces" {
		t.Errorf("QUOTED = %q", os.Getenv("QUOTED"))
	}
}

func TestImportEnvFileMissingIsNotAnError(t *testing.T) {
	if err := ImportEnvFile(filepath.Join(t.TempDir(), "nope.env")); err != nil {
		t.Fatalf("missing .env file should not error, got %v", err)
	}
}
