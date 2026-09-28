package remote

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		"git@github.com:camalot/ironstate-playbook-shared.git": KindGit,
		"ssh://git@github.com/camalot/shared.git":              KindGit,
		"https://github.com/camalot/shared.git":                KindGit,
		"git://example.com/shared":                             KindGit,
		"https://example.com/not-a-repo":                       KindLocal,
		`\\fileserver\playbooks\shared`:                        KindLocal,
		"./roles/shared":                                       KindLocal,
		"C:/playbooks/shared":                                  KindLocal,
		"/srv/playbooks/shared":                                KindLocal,
	}
	for in, want := range cases {
		if got := Classify(in); got != want {
			t.Errorf("Classify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSafeSubPathRejectsTraversalAndAbsolute(t *testing.T) {
	root := t.TempDir()
	for _, sub := range []string{"..", "../outside", `..\outside`, "roles/../../outside", "/etc", `\etc`} {
		if _, err := safeSubPath(root, sub); err == nil {
			t.Errorf("safeSubPath(%q) allowed an escaping path", sub)
		}
	}
}

func TestSafeSubPathAllowsNestedPath(t *testing.T) {
	root := t.TempDir()
	got, err := safeSubPath(root, "path/to/role")
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(filepath.Join(root, "path", "to", "role"))
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveLocalDirectory(t *testing.T) {
	base := t.TempDir()
	roleDir := filepath.Join(base, "shared", "roles", "ssh")
	if err := os.MkdirAll(roleDir, 0o750); err != nil {
		t.Fatal(err)
	}

	res, err := Resolve(Spec{Remote: "shared", Path: "roles/ssh"}, base)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != KindLocal {
		t.Errorf("Kind = %q, want local", res.Kind)
	}
	wantDir, _ := filepath.Abs(roleDir)
	if res.Dir != wantDir {
		t.Errorf("Dir = %q, want %q", res.Dir, wantDir)
	}
}

func TestResolveGitClonesIntoCacheOnce(t *testing.T) {
	cache := t.TempDir()
	origRoot, origRun := CacheRoot, RunGit
	t.Cleanup(func() { CacheRoot, RunGit = origRoot, origRun })
	CacheRoot = func() (string, error) { return cache, nil }

	var calls [][]string
	RunGit = func(dir string, args ...string) error {
		calls = append(calls, args)
		// Simulate a real clone creating the destination directory.
		return os.MkdirAll(args[len(args)-1], 0o750)
	}

	spec := Spec{Remote: "git@github.com:camalot/shared.git", Ref: "main"}
	first, err := Resolve(spec, "")
	if err != nil {
		t.Fatal(err)
	}
	if first.Kind != KindGit {
		t.Fatalf("Kind = %q, want git", first.Kind)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 git call, got %d", len(calls))
	}
	if !strings.Contains(strings.Join(calls[0], " "), "--branch main") {
		t.Errorf("clone did not pass the ref: %v", calls[0])
	}

	second, err := Resolve(spec, "")
	if err != nil {
		t.Fatal(err)
	}
	if second.Dir != first.Dir {
		t.Errorf("cache key not stable: %q vs %q", second.Dir, first.Dir)
	}
	if len(calls) != 1 {
		t.Errorf("an existing checkout was re-cloned: %d calls", len(calls))
	}
}

func TestResolveRejectsOptionLikeRemote(t *testing.T) {
	if _, err := fetchGit("--upload-pack=touch /tmp/pwned", ""); err == nil {
		t.Fatal("expected an option-like remote to be rejected")
	}
	if _, err := fetchGit("git@github.com:camalot/shared.git", "--foo"); err == nil {
		t.Fatal("expected an option-like ref to be rejected")
	}
}

func TestNeedsConfirmation(t *testing.T) {
	if !NeedsConfirmation(KindGit, false) {
		t.Error("a non-isolated remote import must be confirmed")
	}
	if NeedsConfirmation(KindGit, true) {
		t.Error("an isolated remote import must not prompt")
	}
	if NeedsConfirmation(KindLocal, false) {
		t.Error("a local import must not prompt")
	}
}
