package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/facts"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/template"
)

// realModuleNames mirrors ironstate.ps1's Get-PackageManagerHandlers
// registration list (docs/plans/go-rewrite.md §2) — used to flatten the
// *real* repo's site.yml against, as an integration smoke test that this
// port doesn't choke on real content ahead of internal/engine existing.
var realModuleNames = []string{
	"winget", "chocolatey", "gem", "pipx", "npm", "cargo", "go", "eget",
	"git", "cron", "cron_unix", "cron_file", "iptables", "ufw", "advfirewall", "firewall", "zip", "symlinks", "file", "copy", "template", "shell", "blockinfile", "lineinfile",
	"ssh_host_block", "log", "fail", "path", "fact", "registry", "scheduled_task",
	"assert",
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// internal/tasks -> internal -> repo root
	return filepath.Dir(filepath.Dir(wd))
}

func TestFlattenRealSiteYAML(t *testing.T) {
	root := repoRoot(t)
	sitePath := filepath.Join(root, "site.yml")
	if _, err := os.Stat(sitePath); err != nil {
		t.Skipf("site.yml not found at %s (unexpected repo layout): %v", sitePath, err)
	}

	doc, err := packages.LoadFile(sitePath, root)
	if err != nil {
		t.Fatalf("LoadFile(site.yml) error: %v", err)
	}
	docMap := model.AsMap(doc)

	f := facts.Gather()
	v := model.Vars(docMap)
	fset := filters.New()

	// Mirrors ironstate.ps1's whole-document '-Soft' pass before flattening.
	ctx := map[string]any{"facts": f, "vars": v}
	if err := template.ResolveInPlace(docMap, ctx, fset, "site", true); err != nil {
		t.Fatalf("soft-resolving site.yml failed: %v", err)
	}

	taskList, err := model.TaskList(docMap)
	if err != nil {
		t.Fatalf("TaskList(site.yml) error: %v", err)
	}

	leaves, err := Expand(taskList, Options{
		ModuleNames:  realModuleNames,
		PackagesRoot: root,
		Facts:        f,
		Vars:         v,
		Filters:      fset,
	})
	if err != nil {
		t.Fatalf("Expand(site.yml) error: %v", err)
	}

	if len(leaves) == 0 {
		t.Fatal("expected at least one flattened leaf from the real site.yml + its includes")
	}
	for _, l := range leaves {
		if l.Module == "" {
			t.Errorf("leaf with no module: %#v", l)
		}
	}
	t.Logf("flattened %d leaves from the real site.yml", len(leaves))
}

// TestFlattenRealSiteYAMLWithHostOverlay pulls in hosts/krayt.yml ->
// hosts/camalot/main.yml, which 'include's essentially the entire
// roles/*/packages/* tree (regardless of each include's own 'when' —
// that's deferred to dispatch, not evaluated at flatten time) — the
// broadest real-content stress test available for this port without a
// real internal/engine yet.
func TestFlattenRealSiteYAMLWithHostOverlay(t *testing.T) {
	root := repoRoot(t)
	sitePath := filepath.Join(root, "site.yml")
	if _, err := os.Stat(sitePath); err != nil {
		t.Skipf("site.yml not found at %s: %v", sitePath, err)
	}

	t.Setenv("COMPUTERNAME", "KRAYT")
	t.Setenv("USERNAME", "nonexistent-test-user")

	f := facts.Gather()
	doc, err := packages.LoadHierarchy(sitePath, f)
	if err != nil {
		t.Fatalf("LoadHierarchy error: %v", err)
	}
	docMap := model.AsMap(doc)

	v := model.Vars(docMap)
	fset := filters.New()

	ctx := map[string]any{"facts": f, "vars": v}
	if err := template.ResolveInPlace(docMap, ctx, fset, "site", true); err != nil {
		t.Fatalf("soft-resolving site.yml+hosts/krayt.yml failed: %v", err)
	}

	taskList, err := model.TaskList(docMap)
	if err != nil {
		t.Fatalf("TaskList error: %v", err)
	}

	leaves, err := Expand(taskList, Options{
		ModuleNames:  realModuleNames,
		PackagesRoot: root,
		Facts:        f,
		Vars:         v,
		Filters:      fset,
	})
	if err != nil {
		t.Fatalf("Expand error: %v", err)
	}
	if len(leaves) < 50 {
		t.Fatalf("flattened only %d leaves, expected the broad roles/* tree (hundreds) to expand", len(leaves))
	}
	for _, l := range leaves {
		if l.Module == "" {
			t.Errorf("leaf with empty module: %#v", l)
		}
	}
	t.Logf("flattened %d leaves from site.yml + hosts/krayt.yml (-> hosts/camalot -> roles/*)", len(leaves))
}
