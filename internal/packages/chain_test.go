package packages

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
)

func TestChainCandidatesAnySubsetAnyOrder(t *testing.T) {
	facts := map[string]any{
		"computer_name": "krayt",
		"arch":          "amd64",
		"os_family":     "linux",
	}
	got := ChainCandidates(facts)
	// Every permutation of every non-empty subset of {hostname, os_family,
	// arch} (distinct values here, so no name collisions), ordered by
	// priority weight ascending (hostname=8, os_family=4, arch=1 - so e.g.
	// "krayt" alone (8) outranks "amd64.linux" (1+4=5)); same-weight
	// permutations tie-break alphabetically.
	want := []string{
		"amd64", "linux", "amd64.linux", "linux.amd64",
		"krayt", "amd64.krayt", "krayt.amd64",
		"krayt.linux", "linux.krayt",
		"amd64.krayt.linux", "amd64.linux.krayt",
		"krayt.amd64.linux", "krayt.linux.amd64",
		"linux.amd64.krayt", "linux.krayt.amd64",
	}
	if len(got) != len(want) {
		t.Fatalf("ChainCandidates = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ChainCandidates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChainCandidatesTwoCharacteristics(t *testing.T) {
	facts := map[string]any{"computer_name": "krayt", "arch": "amd64"}
	got := ChainCandidates(facts)
	want := []string{"amd64", "krayt", "amd64.krayt", "krayt.amd64"}
	if len(got) != len(want) {
		t.Fatalf("ChainCandidates = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("ChainCandidates[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChainCandidatesEmptyHostnameStillYieldsNonHostnameCandidates(t *testing.T) {
	// Hostname is now an optional segment, not a mandatory leading one -
	// an arch/os_family/platform-only overlay (e.g. 'windows.yml') should
	// still apply even when there's no hostname to anchor a chain on.
	got := ChainCandidates(map[string]any{"arch": "amd64"})
	want := []string{"amd64"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("ChainCandidates = %#v, want %#v", got, want)
	}
}

func TestChainCandidatesEmptyFactsYieldsNone(t *testing.T) {
	if got := ChainCandidates(map[string]any{}); len(got) != 0 {
		t.Fatalf("ChainCandidates = %#v, want none (no characteristics at all)", got)
	}
}

func TestLoadHierarchyAppliesBareOSFamilyOverlayWithoutHostname(t *testing.T) {
	// Real-world scenario: hosts/windows.yml and variables/windows.yml,
	// with no hostname component at all, should still apply on any
	// Windows machine.
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), `
tasks:
  - name: base task
    log: { message: base }
`)
	writeFile(t, filepath.Join(dir, "hosts", "windows.yml"), `
tasks:
  - name: windows host task
    log: { message: windows-host }
`)
	writeFile(t, filepath.Join(dir, "variables", "windows.yml"), `
vars:
  editor: notepad
tasks:
  - name: windows vars task
    log: { message: windows-vars }
`)

	hostFacts := map[string]any{"computer_name": "some-other-host", "os_family": "windows", "platform": "windows"}
	doc, err := LoadHierarchy(filepath.Join(dir, "site.yml"), hostFacts)
	if err != nil {
		t.Fatal(err)
	}
	m := doc.(map[string]any)
	tasks := m["tasks"].([]any)
	if len(tasks) != 3 {
		t.Fatalf("tasks = %#v, want base + hosts/windows.yml + variables/windows.yml appended", tasks)
	}
	if m["vars"].(map[string]any)["editor"] != "notepad" {
		t.Fatalf("vars.editor = %v, want variables/windows.yml's value", m["vars"].(map[string]any)["editor"])
	}
}

func TestLoadHierarchyAcceptsOverlayNameInAnyOrder(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), `
vars:
  editor: code
tasks: []
`)
	// Characteristics written arch-then-hostname, not the "canonical"
	// hostname-then-arch order - should still be recognized.
	writeFile(t, filepath.Join(dir, "variables", "amd64.krayt.yml"), `
vars:
  editor: any-order-editor
`)

	hostFacts := map[string]any{"computer_name": "krayt", "arch": "amd64"}
	doc, err := LoadHierarchy(filepath.Join(dir, "site.yml"), hostFacts)
	if err != nil {
		t.Fatal(err)
	}
	if got := doc.(map[string]any)["vars"].(map[string]any)["editor"]; got != "any-order-editor" {
		t.Errorf("vars.editor = %v, want 'amd64.krayt.yml' (arch-before-hostname order) to be recognized", got)
	}
}

func TestLoadHierarchyDefaultThenChainedOverlaysMostSpecificWins(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), `
vars:
  editor: code
tasks:
  - name: base task
    log: { message: base }
`)
	writeFile(t, filepath.Join(dir, "variables", "main.yml"), `
vars:
  editor: default-editor
  theme: light
tasks:
  - name: default vars task
    log: { message: default }
`)
	writeFile(t, filepath.Join(dir, "variables", "krayt.yml"), `
vars:
  editor: host-editor
`)
	writeFile(t, filepath.Join(dir, "variables", "krayt.amd64.yml"), `
vars:
  editor: host-arch-editor
`)

	hostFacts := map[string]any{"computer_name": "krayt", "arch": "amd64"}
	doc, err := LoadHierarchy(filepath.Join(dir, "site.yml"), hostFacts)
	if err != nil {
		t.Fatal(err)
	}
	m := doc.(map[string]any)
	vars := m["vars"].(map[string]any)
	if vars["editor"] != "host-arch-editor" {
		t.Errorf("vars.editor = %v, want the most specific chained overlay to win", vars["editor"])
	}
	if vars["theme"] != "light" {
		t.Errorf("vars.theme = %v, want the default main.yml's value preserved", vars["theme"])
	}
	tasks := m["tasks"].([]any)
	if len(tasks) != 2 {
		t.Fatalf("tasks = %#v, want base task + variables/main.yml's task appended", tasks)
	}
}

func TestLoadIncludedPackageAppliesChainOverlay(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "cli", "main.yml"), `
vars:
  state: present
tasks:
  - name: base
    winget: { package: base-pkg, state: "${{ vars.state }}" }
`)
	writeFile(t, filepath.Join(root, "cli", "krayt.yml"), `
vars:
  state: absent
`)

	facts := map[string]any{"computer_name": "krayt"}
	included, err := LoadIncludedPackage(map[string]any{"name": "cli"}, root, facts, map[string]any{}, filters.New())
	if err != nil {
		t.Fatal(err)
	}
	data := included.Data.(map[string]any)
	if data["vars"].(map[string]any)["state"] != "absent" {
		t.Errorf("vars.state = %v, want the chained overlay to win", data["vars"].(map[string]any)["state"])
	}
}

func TestResolvePlaybookPathExactFile(t *testing.T) {
	dir := t.TempDir()
	sitePath := filepath.Join(dir, "custom.yml")
	writeFile(t, sitePath, "---\nvars: {}\ntasks: []\n")

	got, err := ResolvePlaybookPath(sitePath)
	if err != nil {
		t.Fatal(err)
	}
	if got != sitePath {
		t.Errorf("ResolvePlaybookPath = %q, want %q", got, sitePath)
	}
}

func TestResolvePlaybookPathDirectoryDefaultsToSiteYML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), "---\nvars: {}\ntasks: []\n")

	got, err := ResolvePlaybookPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "site.yml"); got != want {
		t.Errorf("ResolvePlaybookPath = %q, want %q", got, want)
	}
}

func TestResolvePlaybookPathDirectoryFallsBackToMainYML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.yml"), "---\nvars: {}\ntasks: []\n")

	got, err := ResolvePlaybookPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "main.yml"); got != want {
		t.Errorf("ResolvePlaybookPath = %q, want %q", got, want)
	}
}

func TestResolvePlaybookPathBareNameFindsSiblingYAML(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "camalot.yml"), "---\nvars: {}\ntasks: []\n")

	input := filepath.Join(dir, "camalot")
	got, err := ResolvePlaybookPath(input)
	if err != nil {
		t.Fatal(err)
	}
	if want := input + ".yml"; got != want {
		t.Errorf("ResolvePlaybookPath = %q, want %q", got, want)
	}
}

func TestResolvePlaybookPathErrorsWhenNothingFound(t *testing.T) {
	dir := t.TempDir()
	if _, err := ResolvePlaybookPath(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("expected an error when no playbook can be located")
	}
}

func TestResolvePlaybookPathDefaultsToCWD(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "site.yml"), "---\nvars: {}\ntasks: []\n")

	origWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(origWD) }()

	got, err := ResolvePlaybookPath("")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(".", "site.yml"); got != want {
		t.Errorf("ResolvePlaybookPath(\"\") = %q, want %q", got, want)
	}
}
