package tasks

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
)

var moduleNames = []string{"winget", "chocolatey", "log", "copy", "shell", "fact", "assert"}

func parseTasks(t *testing.T, yaml string) []any {
	t.Helper()
	doc, err := model.Unmarshal([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	list, err := model.TaskList(doc)
	if err != nil {
		t.Fatal(err)
	}
	return list
}

func expandOK(t *testing.T, yaml string, opts Options) []Leaf {
	t.Helper()
	if opts.ModuleNames == nil {
		opts.ModuleNames = moduleNames
	}
	if opts.Filters == nil {
		opts.Filters = filters.New()
	}
	leaves, err := Expand(parseTasks(t, yaml), opts)
	if err != nil {
		t.Fatalf("Expand error: %v", err)
	}
	return leaves
}

func TestBasicLeafFlattening(t *testing.T) {
	leaves := expandOK(t, `
- name: install age
  tags: [security, cli]
  winget: { package: FiloSottile.age, state: present }
`, Options{})
	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v", leaves)
	}
	l := leaves[0]
	if l.Module != "winget" || l.Name != "install age" {
		t.Fatalf("leaf = %#v", l)
	}
	if !reflect.DeepEqual(l.Tags, []string{"security", "cli"}) {
		t.Fatalf("tags = %#v", l.Tags)
	}
	if l.Item["package"] != "FiloSottile.age" {
		t.Fatalf("item.package = %v", l.Item["package"])
	}
}

func TestTagsCascadeAndDedupe(t *testing.T) {
	leaves := expandOK(t, `
- name: group
  tags: [cli]
  actions:
    - name: leaf
      tags: [cli, security]
      log: { message: hi }
`, Options{})
	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v", leaves)
	}
	if !reflect.DeepEqual(leaves[0].Tags, []string{"cli", "security"}) {
		t.Fatalf("tags = %#v, want cascaded+deduped", leaves[0].Tags)
	}
}

func TestWhenAccumulatesAsAppendedList(t *testing.T) {
	leaves := expandOK(t, `
- name: group
  when: "is_admin == true"
  actions:
    - name: leaf
      when: "computer_name == 'KRAYT'"
      log: { message: hi }
`, Options{})
	want := []any{"is_admin == true", "computer_name == 'KRAYT'"}
	if !reflect.DeepEqual(leaves[0].When, want) {
		t.Fatalf("when = %#v, want %#v", leaves[0].When, want)
	}
}

func TestBooleanWhenLiteralIsKeptAsBool(t *testing.T) {
	leaves := expandOK(t, `
- name: leaf
  when: true
  log: { message: hi }
`, Options{})
	if len(leaves[0].When) != 1 || leaves[0].When[0] != true {
		t.Fatalf("when = %#v, want [true] (bool, not stringified)", leaves[0].When)
	}
}

func TestItemsLoopMaterializesOneLeafPerEntry(t *testing.T) {
	leaves := expandOK(t, `
- name: install ${{ item.package }}
  id: pkg
  winget:
    package: ${{ item.package }}
    state: ${{ item.state }}
  items:
    - { package: a, state: present }
    - { package: b, state: absent }
`, Options{})
	if len(leaves) != 2 {
		t.Fatalf("leaves = %#v", leaves)
	}
	if leaves[0].Name != "install a" || leaves[0].Item["package"] != "a" || leaves[0].Item["state"] != "present" {
		t.Fatalf("leaf 0 = %#v", leaves[0])
	}
	if leaves[1].Name != "install b" || leaves[1].Item["package"] != "b" || leaves[1].Item["state"] != "absent" {
		t.Fatalf("leaf 1 = %#v", leaves[1])
	}
	if !leaves[0].Looped || !leaves[1].Looped {
		t.Fatalf("expected both loop iterations to carry Looped=true")
	}
	if leaves[0].ID != "pkg" || leaves[1].ID != "pkg" {
		t.Fatalf("expected the same id on every iteration")
	}
}

func TestItemsOmitsUnresolvedFieldRatherThanEmptyString(t *testing.T) {
	leaves := expandOK(t, `
- name: install ${{ item.package }}
  eget_stub:
    args: ${{ item.args }}
  items:
    - { package: a, args: ["--to=x"] }
    - { package: b }
`, Options{ModuleNames: append(append([]string{}, moduleNames...), "eget_stub")})
	if len(leaves) != 2 {
		t.Fatalf("leaves = %#v", leaves)
	}
	if _, present := leaves[1].Item["args"]; present {
		t.Fatalf("leaf 1 item = %#v, want 'args' key omitted entirely", leaves[1].Item)
	}
}

func TestWithMaterializesExactlyOneCopyWithoutIterating(t *testing.T) {
	leaves := expandOK(t, `
- name: ref
  log: { message: "${{ item }}" }
  with: ["a", "b"]
`, Options{})
	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v, want exactly 1 (with never iterates)", leaves)
	}
	// Whole-value '${{ }}' substitution keeps the native type — 'item' is
	// bound to the *entire* 'with' value here, not one element of it.
	want := []any{"a", "b"}
	if !reflect.DeepEqual(leaves[0].Item["message"], want) {
		t.Fatalf("message = %#v, want %#v (the whole 'with' value, untouched)", leaves[0].Item["message"], want)
	}
}

func TestNestedLoopExposesParentItem(t *testing.T) {
	leaves := expandOK(t, `
- name: outer
  items: [{ name: alice, repos: [a, b] }, { name: bob, repos: [c] }]
  actions:
    - name: inner
      items: "${{ item.repos }}"
      log:
        message: "${{ parent.item.name }} owns ${{ item }}"
`, Options{})
	if len(leaves) != 3 {
		t.Fatalf("leaves = %#v, want 3 (2 repos for alice + 1 for bob)", leaves)
	}
	got := []string{
		leaves[0].Item["message"].(string),
		leaves[1].Item["message"].(string),
		leaves[2].Item["message"].(string),
	}
	want := []string{"alice owns a", "alice owns b", "bob owns c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("messages = %#v, want %#v", got, want)
	}
}

func TestUnrecognizedModuleKeyIsSkipped(t *testing.T) {
	leaves := expandOK(t, `
- name: bogus
  not_a_real_module: { x: 1 }
`, Options{})
	if len(leaves) != 0 {
		t.Fatalf("leaves = %#v, want none", leaves)
	}
}

func TestMultipleModuleKeysPicksFirstInModuleNamesOrder(t *testing.T) {
	leaves := expandOK(t, `
- name: ambiguous
  winget: { package: a }
  chocolatey: { package: b }
`, Options{})
	if len(leaves) != 1 || leaves[0].Module != "winget" {
		t.Fatalf("leaves = %#v, want winget to win (first in ModuleNames)", leaves)
	}
}

func TestIncludeLoadsPackageAndIsolatesPackageVars(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "cli"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "cli", "main.yml"), []byte(`
vars:
  local_default: builtin
tasks:
  - name: leaf in package
    log: { message: "${{ local_default }}" }
`), 0o600); err != nil {
		t.Fatal(err)
	}

	leaves := expandOK(t, `
- name: include cli
  tags: [cli]
  include: { name: cli }
`, Options{PackagesRoot: root})

	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v", leaves)
	}
	if leaves[0].Item["message"] != "builtin" {
		t.Fatalf("message = %v, want the package's own local var resolved", leaves[0].Item["message"])
	}
	if !reflect.DeepEqual(leaves[0].Tags, []string{"cli"}) {
		t.Fatalf("tags = %#v, want inherited from the include task", leaves[0].Tags)
	}
}

func TestIncludeWithoutPackagesRootIsSkipped(t *testing.T) {
	leaves := expandOK(t, `
- name: include cli
  include: { name: cli }
`, Options{})
	if len(leaves) != 0 {
		t.Fatalf("leaves = %#v, want none (no PackagesRoot configured)", leaves)
	}
}

func TestMissingIncludeIsSkippedNotErrored(t *testing.T) {
	root := t.TempDir()
	leaves := expandOK(t, `
- name: include missing
  include: { name: does-not-exist }
`, Options{PackagesRoot: root})
	if len(leaves) != 0 {
		t.Fatalf("leaves = %#v, want none", leaves)
	}
}

func TestFailedWhenAndContinueOnErrorCarryThrough(t *testing.T) {
	leaves := expandOK(t, `
- name: leaf
  id: check
  failed_when: "rc != 0"
  continue_on_error: true
  log: { message: hi }
`, Options{})
	l := leaves[0]
	if l.ID != "check" || !l.ContinueOnError {
		t.Fatalf("leaf = %#v", l)
	}
	if !reflect.DeepEqual(l.FailedWhen, []any{"rc != 0"}) {
		t.Fatalf("failed_when = %#v", l.FailedWhen)
	}
}

// Not part of Tasks.psm1 itself, but exercises the include path through
// internal/packages end-to-end to catch integration drift between the two
// packages early.
func TestIncludeIntegrationWithRealPackagesLoader(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "roles", "demo"), 0o750)
	_ = os.WriteFile(filepath.Join(root, "roles", "demo", "main.yml"), []byte(`
tasks:
  - name: nested
    log: { message: "${{ package.name }}" }
`), 0o600)

	leaves := expandOK(t, `
- include: { name: roles/demo }
`, Options{PackagesRoot: root})
	if len(leaves) != 1 || leaves[0].Item["message"] != "roles/demo" {
		t.Fatalf("leaves = %#v", leaves)
	}
	_ = packages.Included{} // sanity: package compiles/links against tasks' usage
}
