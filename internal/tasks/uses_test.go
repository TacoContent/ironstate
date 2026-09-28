package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/remote"
)

// writeSharedPlaybook creates a minimal usable playbook fragment
// under <base>/shared/roles/greet and returns <base>.
func writeSharedPlaybook(t *testing.T, body string) string {
	t.Helper()
	base := t.TempDir()
	dir := filepath.Join(base, "shared", "roles", "greet")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.yml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return base
}

func TestUsesLocalPathExpandsTasks(t *testing.T) {
	base := writeSharedPlaybook(t, `
tasks:
  - name: greet
    log: { message: hello }
`)

	leaves := expandOK(t, `
- name: A Local Role
  uses:
    remote: shared
    path: roles/greet
`, Options{PackagesRoot: base})

	if len(leaves) != 1 || leaves[0].Module != "log" {
		t.Fatalf("leaves = %#v", leaves)
	}
	if leaves[0].Isolated {
		t.Error("a local source without 'isolate' must not be isolated")
	}
}

func TestUsesIsolateRestrictsContextToWith(t *testing.T) {
	base := writeSharedPlaybook(t, `
tasks:
  - name: greet
    log: { message: hi }
`)

	leaves := expandOK(t, `
- name: A Sandboxed Role
  uses:
    remote: shared
    path: roles/greet
    isolate: true
    with:
      vars: { greeting: bonjour }
      facts: { platform: linux }
`, Options{
		PackagesRoot: base,
		Facts:        map[string]any{"platform": "windows", "secret_fact": "leaked"},
		Vars:         map[string]any{"token": "leaked"},
	})

	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v", leaves)
	}
	l := leaves[0]
	if !l.Isolated {
		t.Fatal("a 'uses' with 'isolate: true' must mark its leaves isolated")
	}
	if l.IsolatedVars["greeting"] != "bonjour" {
		t.Errorf("isolated vars = %#v", l.IsolatedVars)
	}
	if _, leaked := l.IsolatedVars["token"]; leaked {
		t.Error("site vars leaked into an isolated 'uses'")
	}
	if _, leaked := l.IsolatedFacts["secret_fact"]; leaked {
		t.Error("host facts leaked into an isolated 'uses'")
	}
	if l.IsolatedFacts["platform"] != "linux" {
		t.Errorf("isolated facts = %#v", l.IsolatedFacts)
	}
}

func TestUsesPropagatesTagsAndWhen(t *testing.T) {
	base := writeSharedPlaybook(t, `
tasks:
  - name: greet
    tags: [inner]
    when: [inner_condition]
    log: { message: hi }
`)

	leaves := expandOK(t, `
- name: A Local Role
  tags: [outer]
  when:
    - outer_condition
  uses:
    remote: shared
    path: roles/greet
`, Options{PackagesRoot: base})

	if len(leaves) != 1 {
		t.Fatalf("leaves = %#v", leaves)
	}
	if got := leaves[0].Tags; len(got) != 2 || got[0] != "outer" || got[1] != "inner" {
		t.Errorf("tags = %#v", got)
	}
	if got := leaves[0].When; len(got) != 2 || got[0] != "outer_condition" || got[1] != "inner_condition" {
		t.Errorf("when = %#v", got)
	}
}

func TestUsesRemoteNonIsolatedPromptsAndSkipsWhenDeclined(t *testing.T) {
	origConfirm, origCacheRoot := remote.Confirm, remote.CacheRoot
	t.Cleanup(func() { remote.Confirm, remote.CacheRoot = origConfirm, origCacheRoot })

	cache := t.TempDir()
	remote.CacheRoot = func() (string, error) { return cache, nil }

	prompted := false
	remote.Confirm = func(kind remote.Kind, source string) (bool, error) {
		prompted = true
		return false, nil
	}

	leaves := expandOK(t, `
- name: A Remote Role
  uses:
    remote: git@github.com:camalot/ironstate-playbook-shared.git
    path: roles/greet
`, Options{PackagesRoot: t.TempDir()})

	if !prompted {
		t.Fatal("a remote, non-isolated 'uses' must prompt for confirmation")
	}
	if len(leaves) != 0 {
		t.Fatalf("a declined source must contribute no leaves: %#v", leaves)
	}
}

func TestUsesRemoteAllowRemoteSkipsPrompt(t *testing.T) {
	stubGitClone(t)
	remote.Confirm = func(kind remote.Kind, source string) (bool, error) {
		t.Error("--allow-remote-uses must suppress the prompt")
		return false, nil
	}

	leaves := expandOK(t, `
- name: A Remote Role
  uses:
    remote: git@github.com:camalot/ironstate-playbook-shared.git
    path: roles/greet
`, Options{PackagesRoot: t.TempDir(), AllowRemoteUses: true})

	if len(leaves) != 1 || leaves[0].Module != "log" {
		t.Fatalf("leaves = %#v", leaves)
	}
}

// TestUsesTrustedSkipsPrompt covers the per-source counterpart to
// --allow-remote-uses: a source the playbook itself marks trusted is
// fetched and run without asking.
func TestUsesTrustedSkipsPrompt(t *testing.T) {
	stubGitClone(t)
	remote.Confirm = func(kind remote.Kind, source string) (bool, error) {
		t.Error("'trusted: true' must suppress the prompt")
		return false, nil
	}

	leaves := expandOK(t, `
- name: A Trusted Remote Role
  uses:
    remote: git@github.com:camalot/ironstate-playbook-shared.git
    path: roles/greet
    trusted: true
`, Options{PackagesRoot: t.TempDir()})

	if len(leaves) != 1 || leaves[0].Module != "log" {
		t.Fatalf("leaves = %#v", leaves)
	}
	if leaves[0].Isolated {
		t.Error("'trusted' must not imply isolation")
	}
}

// stubGitClone redirects the remote cache to a temp dir and replaces the
// real clone with one that writes a minimal usable role.
func stubGitClone(t *testing.T) {
	t.Helper()
	origConfirm, origCacheRoot, origRunGit := remote.Confirm, remote.CacheRoot, remote.RunGit
	t.Cleanup(func() {
		remote.Confirm, remote.CacheRoot, remote.RunGit = origConfirm, origCacheRoot, origRunGit
	})

	cache := t.TempDir()
	remote.CacheRoot = func() (string, error) { return cache, nil }
	remote.RunGit = func(dir string, args ...string) error {
		dest := args[len(args)-1]
		roleDir := filepath.Join(dest, "roles", "greet")
		if err := os.MkdirAll(roleDir, 0o750); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(roleDir, "main.yml"), []byte("tasks:\n  - name: greet\n    log: { message: hi }\n"), 0o600)
	}
}

func TestUsesMissingRemoteWarnsAndSkips(t *testing.T) {
	origWarn := packages.Warn
	t.Cleanup(func() { packages.Warn = origWarn })

	warned := false
	packages.Warn = func(format string, args ...any) { warned = true }

	leaves := expandOK(t, `
- name: Broken
  uses:
    path: roles/greet
`, Options{PackagesRoot: t.TempDir()})

	if !warned {
		t.Error("a 'uses' with no 'remote' must warn")
	}
	if len(leaves) != 0 {
		t.Fatalf("leaves = %#v", leaves)
	}
}
