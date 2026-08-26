package handlers

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"

	"github.com/TacoContent/ironstate/internal/engine"
)

func testCtx() engine.Context {
	return engine.Context{Flat: map[string]any{}, Apply: true}
}

func TestLogHandlerFlatShorthandInstall(t *testing.T) {
	h := logHandler{}
	item := map[string]any{"message": "hello", "level": "info"}

	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (state defaults present)", installed, err)
	}
	desc, err := h.Describe(item, engine.ActionInstall, testCtx())
	if err != nil || desc != "log (install): hello" {
		t.Fatalf("desc=%q err=%v", desc, err)
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
}

func TestLogHandlerAbsentStateAlwaysUninstalls(t *testing.T) {
	h := logHandler{}
	item := map[string]any{"state": "absent", "uninstall": map[string]any{"message": "bye"}}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (state=absent always 'installed')", installed, err)
	}
}

func TestLogHandlerNoConfigAtAllAlwaysRuns(t *testing.T) {
	h := logHandler{}
	// Nothing scoped at all (no 'message', no 'install'/'uninstall'
	// section) - not scoped to either phase specifically, so it still
	// runs (matching log's "no real idempotent 'already applied'
	// concept"); this is also what a message that resolved to nothing
	// (e.g. an unresolved '${{ }}' reference, omitted entirely) looks
	// like by the time the handler sees it - it should still run, not
	// silently skip.
	installed, err := h.Test(map[string]any{}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (runs, even with nothing to say)", installed, err)
	}
}

func TestLogHandlerAbsentStateWithNoUninstallMessageSkips(t *testing.T) {
	h := logHandler{}
	// An explicit but empty 'uninstall' section blocks falling back to
	// any flat default - present-but-empty means "nothing to log here".
	item := map[string]any{"state": "absent", "uninstall": map[string]any{}}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (skip: 'uninstall' section present but has no message)", installed, err)
	}
}

func TestLogHandlerInstallOnlyMissingFallsBackToDefault(t *testing.T) {
	h := logHandler{}
	item := map[string]any{"message": "default message", "uninstall": map[string]any{"message": "bye"}}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (install phase falls back to the flat default)", installed, err)
	}
	desc, err := h.Describe(item, engine.ActionInstall, testCtx())
	if err != nil || desc != "log (install): default message" {
		t.Fatalf("desc=%q err=%v", desc, err)
	}
}

func TestLogHandlerUninstallOnlySkipsInstallPhase(t *testing.T) {
	h := logHandler{}
	// No 'install' section, no flat default message - only 'uninstall' -
	// so this log only ever runs when state is 'absent'.
	item := map[string]any{"uninstall": map[string]any{"message": "bye"}}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (skip: install-phase has nothing to say)", installed, err)
	}

	item["state"] = "absent"
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true ('absent' + has an uninstall message -> uninstall runs)", installed, err)
	}
}

func TestAssertHandlerPassAndFail(t *testing.T) {
	h := assertHandler{}
	installed, err := h.Test(nil, "", testCtx())
	if err != nil || installed {
		t.Fatalf("assert Test must always report false, got %v err=%v", installed, err)
	}

	ctx := engine.Context{Flat: map[string]any{"a": true}, Apply: true}
	exec, err := h.Install(map[string]any{"that": []any{"a == true"}}, "check", ctx)
	if err != nil || exec.RC != 0 {
		t.Fatalf("expected pass, got exec=%#v err=%v", exec, err)
	}

	exec, err = h.Install(map[string]any{"that": []any{"a == false"}}, "check", ctx)
	if err != nil || exec.RC != 1 {
		t.Fatalf("expected fail (rc=1), got exec=%#v err=%v", exec, err)
	}
}

func TestFileHandlerCreatesAndRemovesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "file.txt")
	h := fileHandler{}
	item := map[string]any{"path": path, "type": "file"}

	installed, _ := h.Test(item, "", testCtx())
	if installed {
		t.Fatal("expected not installed before creation")
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	installed, _ = h.Test(item, "", testCtx())
	if !installed {
		t.Fatal("expected installed after creation")
	}

	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("expected file removed")
	}
}

func TestFileHandlerDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "adir")
	h := fileHandler{}
	item := map[string]any{"path": path, "type": "directory"}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, _ := h.Test(item, "", testCtx())
	if !installed {
		t.Fatal("expected directory installed")
	}
}

func TestFileHandlerSymlink(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	h := fileHandler{}
	item := map[string]any{"path": link, "type": "link", "src": src}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
}

func TestSymlinksHandlerDelegatesToFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("hi"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "dest.txt")
	h := symlinksHandler{}
	item := map[string]any{"src": src, "dest": dest}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
}

func TestCopyHandlerSingleFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.txt")
	if err := os.WriteFile(src, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "out", "dest.txt")
	h := copyHandler{}
	item := map[string]any{"src": src, "dest": dest}

	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true after copy", installed, err)
	}

	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if fileExists(dest) {
		t.Fatal("expected dest removed after uninstall")
	}
}

func TestCopyHandlerDirectoryRecursive(t *testing.T) {
	dir := t.TempDir()
	srcDir := filepath.Join(dir, "srcdir")
	if err := os.MkdirAll(filepath.Join(srcDir, "nested"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "nested", "b.txt"), []byte("b"), 0o600); err != nil {
		t.Fatal(err)
	}

	destRoot := filepath.Join(dir, "dest")
	h := copyHandler{}
	item := map[string]any{"src": srcDir, "dest": destRoot}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	// no trailing slash -> nests under dest/srcdir/...
	if !fileExists(filepath.Join(destRoot, "srcdir", "a.txt")) {
		t.Fatal("expected a.txt copied under dest/srcdir")
	}
	if !fileExists(filepath.Join(destRoot, "srcdir", "nested", "b.txt")) {
		t.Fatal("expected nested/b.txt copied")
	}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
}

func TestBlockInFileInsertsAndUpdatesBlock(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "profile.ps1")
	h := blockInFileHandler{}
	item := map[string]any{"dest": dest, "create": true, "block": "line1\nline2"}

	installed, _ := h.Test(item, "task", testCtx())
	if installed {
		t.Fatal("expected not installed before write")
	}
	if _, err := h.Install(item, "task", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, err := h.Test(item, "task", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	content := string(data)
	if !strings.Contains(content, "line1") || !strings.Contains(content, "IRONSTATE MANAGED - task") {
		t.Fatalf("unexpected content: %q", content)
	}

	// Update the block content and verify replace-in-place, not duplicate markers.
	item["block"] = "line1\nline2\nline3"
	if _, err := h.Install(item, "task", testCtx()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if strings.Count(string(data), "BEGIN") != 1 {
		t.Fatalf("expected exactly one BEGIN marker after update, content=%q", string(data))
	}

	if _, err := h.Uninstall(item, "task", testCtx()); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if strings.Contains(string(data), "line1") {
		t.Fatalf("expected block removed, content=%q", string(data))
	}
}

func TestLineInFileReplaceAndRemove(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config.txt")
	if err := os.WriteFile(dest, []byte("A=1\nB=2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}
	item := map[string]any{
		"path":   dest,
		"regexp": "^B=",
		"line":   "B=3",
	}

	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before replacement", installed, err)
	}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "A=1\nB=3\n" {
		t.Fatalf("unexpected content after replace: %q", string(data))
	}

	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true after replacement", installed, err)
	}

	remove := map[string]any{"path": dest, "state": "absent", "regexp": "^B="}
	if _, err := h.Uninstall(remove, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ = os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "A=1\n" {
		t.Fatalf("unexpected content after absent/remove: %q", string(data))
	}
}

func TestLineInFileInsertBeforeAndAfter(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "profile.txt")
	if err := os.WriteFile(dest, []byte("one\nneedle\ntwo\nneedle\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}

	if _, err := h.Install(map[string]any{
		"path":        dest,
		"line":        "inserted-after",
		"insertafter": "needle",
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	if _, err := h.Install(map[string]any{
		"path":         dest,
		"line":         "inserted-before",
		"insertbefore": "needle",
		"firstmatch":   true,
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	got := string(data)
	if !strings.Contains(got, "inserted-before\nneedle") {
		t.Fatalf("expected first-match insertbefore placement, content=%q", got)
	}
	if !strings.Contains(got, "needle\ninserted-after\nthree") {
		t.Fatalf("expected last-match insertafter placement, content=%q", got)
	}
}

func TestLineInFileBackrefs(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "service.conf")
	if err := os.WriteFile(dest, []byte("listen=127.0.0.1:8080\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}
	if _, err := h.Install(map[string]any{
		"path":     dest,
		"regexp":   "^(listen=).*$",
		"line":     "\\1localhost:9090",
		"backrefs": true,
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "listen=localhost:9090\n" {
		t.Fatalf("unexpected backref content: %q", string(data))
	}
}

func TestLineInFileBackrefsDollarSyntax(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "service.conf")
	if err := os.WriteFile(dest, []byte("version: 0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}
	if _, err := h.Install(map[string]any{
		"path":     dest,
		"regexp":   "^([Vv]ersion):\\s+.*$",
		"line":     "$1: 1.2.5",
		"backrefs": true,
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "version: 1.2.5\n" {
		t.Fatalf("unexpected $1 backref content: %q", string(data))
	}
}

func TestLineInFileTemplateWithGoTemplate(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "version.txt")
	if err := os.WriteFile(dest, []byte("version: 0.1.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}
	if _, err := h.Install(map[string]any{
		"path":   dest,
		"regexp": "^version:",
		"line":   "Version: {{ .Version }}",
		"with":   map[string]any{"Version": "1.2.5"},
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "Version: 1.2.5\n" {
		t.Fatalf("unexpected gotemplate replacement content: %q", string(data))
	}
}

func TestLineInFileTemplateWithTemplateSyntax(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "version.txt")
	if err := os.WriteFile(dest, []byte("version: old\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := lineInFileHandler{}
	if _, err := h.Install(map[string]any{
		"path":   dest,
		"regexp": "^version:",
		"line":   "Version: ${{ input.Version }}",
		"with":   map[string]any{"Version": "1.2.5"},
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	if string(data) != "Version: 1.2.5\n" {
		t.Fatalf("unexpected ${{ }} replacement content: %q", string(data))
	}
}

func TestGitHandlerLatestWithUpdateAlwaysRuns(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	h := gitHandler{}
	installed, err := h.Test(map[string]any{
		"repo":   "https://example.com/org/repo.git",
		"dest":   dest,
		"state":  "latest",
		"update": true,
	}, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if installed {
		t.Fatal("expected latest+update to report not-installed so Install runs")
	}
}

func TestGitHandlerPresentAtVersionReportsInstalled(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		if exe != "git" {
			return ironexec.Result{}, nil
		}
		if len(args) >= 5 && args[2] == "config" && args[3] == "--get" && args[4] == "remote.origin.url" {
			return ironexec.Result{RC: 0, Stdout: "https://example.com/org/repo.git\n", StdoutLines: []string{"https://example.com/org/repo.git"}}, nil
		}
		if len(args) >= 5 && args[2] == "rev-parse" {
			return ironexec.Result{RC: 0, Stdout: "abc123\n", StdoutLines: []string{"abc123"}}, nil
		}
		return ironexec.Result{RC: 0}, nil
	})

	dir := t.TempDir()
	dest := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	h := gitHandler{}
	installed, err := h.Test(map[string]any{
		"repo":    "https://example.com/org/repo.git",
		"dest":    dest,
		"version": "v1.2.3",
	}, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if !installed {
		t.Fatal("expected matching HEAD/version refs to report installed")
	}
}

func TestGitHandlerCloneCommandIncludesBranchDepthAndSingleBranch(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()

	var calls [][]string
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		calls = append(calls, append([]string{exe}, args...))
		return ironexec.Result{RC: 0}, nil
	})

	h := gitHandler{}
	dir := t.TempDir()
	dest := filepath.Join(dir, "repo")
	if _, err := h.Install(map[string]any{
		"repo":          "https://example.com/org/repo.git",
		"dest":          dest,
		"ref":           "v1.2.3",
		"depth":         1,
		"single_branch": true,
		"recursive":     false,
	}, "", testCtx()); err != nil {
		t.Fatal(err)
	}

	if len(calls) == 0 {
		t.Fatal("expected at least one git command invocation")
	}
	first := calls[0]
	joined := strings.Join(first, " ")
	if !strings.Contains(joined, "git clone") || !strings.Contains(joined, "--depth 1") || !strings.Contains(joined, "--single-branch") || !strings.Contains(joined, "--branch v1.2.3") {
		t.Fatalf("unexpected clone invocation: %q", joined)
	}
}

func TestGitHandlerUninstallRemovesCheckout(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "repo")
	if err := os.MkdirAll(filepath.Join(dest, ".git"), 0o750); err != nil {
		t.Fatal(err)
	}

	h := gitHandler{}
	if _, err := h.Uninstall(map[string]any{"dest": dest}, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if fileExists(dest) {
		t.Fatalf("expected git uninstall to remove %s", dest)
	}
}

func TestIPTablesHandlerBuildsAddRuleCommand(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()

	var calls [][]string
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		calls = append(calls, append([]string{exe}, args...))
		if len(args) > 0 && args[0] == "-C" {
			return ironexec.Result{RC: 1}, nil
		}
		return ironexec.Result{RC: 0}, nil
	})

	h := iptablesHandler{}
	item := map[string]any{
		"chain":    "INPUT",
		"protocol": "tcp",
		"port":     "22",
		"action":   "allow",
	}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 {
		t.Fatal("expected iptables command call")
	}
	joined := strings.Join(calls[len(calls)-1], " ")
	if !strings.Contains(joined, "iptables -A INPUT") || !strings.Contains(joined, "--dport 22") || !strings.Contains(joined, "-j ACCEPT") {
		t.Fatalf("unexpected iptables command: %q", joined)
	}
}

func TestUFWHandlerDeleteTreatsMissingRuleAsSuccess(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		return ironexec.Result{RC: 1, Stderr: "Could not delete non-existent rule\n", StderrLines: []string{"Could not delete non-existent rule"}}, nil
	})

	h := ufwHandler{}
	res, err := h.Uninstall(map[string]any{"rule": "allow", "port": "22"}, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if res.RC != 0 {
		t.Fatalf("expected missing-rule delete to normalize to rc=0, got %d", res.RC)
	}
}

func TestAdvFirewallHandlerRequiresRuleName(t *testing.T) {
	h := advFirewallHandler{}
	_, err := h.Install(map[string]any{}, "", testCtx())
	if err == nil {
		t.Fatal("expected advfirewall install to fail without name")
	}
}

func TestFirewallWrapperExplicitIPTablesBackend(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()

	var calls [][]string
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		calls = append(calls, append([]string{exe}, args...))
		return ironexec.Result{RC: 0}, nil
	})

	h := firewallHandler{}
	_, err := h.Install(map[string]any{
		"backend":   "iptables",
		"direction": "in",
		"protocol":  "tcp",
		"port":      "443",
		"action":    "allow",
	}, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) == 0 {
		t.Fatal("expected translated backend invocation")
	}
	joined := strings.Join(calls[len(calls)-1], " ")
	if !strings.Contains(joined, "iptables") || !strings.Contains(joined, "--dport 443") || !strings.Contains(joined, "-j ACCEPT") {
		t.Fatalf("unexpected translated firewall call: %q", joined)
	}
}

func TestFirewallWrapperRejectsUnknownBackend(t *testing.T) {
	h := firewallHandler{}
	_, err := h.Install(map[string]any{"backend": "unknown"}, "", testCtx())
	if err == nil {
		t.Fatal("expected error for unknown backend")
	}
}

func TestSshHostBlockRendersHostEntry(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")
	h := sshHostBlockHandler{}
	item := map[string]any{
		"dest":   dest,
		"create": true,
		"hosts": []any{
			map[string]any{"host": "example", "user": "alice", "identities_only": true},
		},
	}

	if _, err := h.Install(item, "hosts", testCtx()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(dest) //nolint:gosec // dest is a t.TempDir()-derived path this same test just wrote, not user input
	content := string(data)
	if !strings.Contains(content, "Host example") || !strings.Contains(content, "HostName example") || !strings.Contains(content, "User alice") || !strings.Contains(content, "IdentitiesOnly yes") {
		t.Fatalf("unexpected content: %q", content)
	}

	installed, err := h.Test(item, "hosts", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
}

func TestSshHostBlockTestIgnoresDirectiveOrder(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "config")
	// Hand-authored order (HostName, IdentityFile, User, IdentitiesOnly)
	// deliberately differs from the handler's own alphabetically-sorted
	// generated order, but is otherwise identical ssh_config content.
	existing := "# BEGIN IRONSTATE MANAGED - github-commercial\n" +
		"# Commercial github\n" +
		"Host github.com\n" +
		"  HostName github.com\n" +
		"  IdentityFile ~/.ssh/id_rsa\n" +
		"  User git\n" +
		"  IdentitiesOnly yes\n" +
		"# END IRONSTATE MANAGED - github-commercial\n"
	if err := os.WriteFile(dest, []byte(existing), 0o600); err != nil {
		t.Fatal(err)
	}

	h := sshHostBlockHandler{}
	item := map[string]any{
		"dest":        dest,
		"marker_name": "github-commercial",
		"hosts": []any{
			map[string]any{
				"comment":         "Commercial github",
				"host":            "github.com",
				"identity_file":   "~/.ssh/id_rsa",
				"user":            "git",
				"identities_only": true,
			},
		},
	}

	installed, err := h.Test(item, "hosts", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (same directives, different order)", installed, err)
	}
}

func TestCreatesPresentEmptyIsAlwaysFalse(t *testing.T) {
	if testCreatesPresent(nil) {
		t.Fatal("empty creates should always report not-installed")
	}
}

func TestCreatesPresentGlob(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.exe"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	present := testCreatesPresent([]any{filepath.Join(dir, "*.exe")})
	if !present {
		t.Fatal("expected glob match to report present")
	}
	absent := testCreatesPresent([]any{filepath.Join(dir, "*.missing")})
	if absent {
		t.Fatal("expected no-match glob to report absent")
	}
}

func TestFactHandlerPlainValue(t *testing.T) {
	h := factHandler{}
	item := map[string]any{"name": "greeting", "value": "hi"}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (state defaults present)", installed, err)
	}
	exec, err := h.Install(item, "", testCtx())
	if err != nil || exec.RC != 0 {
		t.Fatalf("exec=%#v err=%v", exec, err)
	}
}

func TestFactHandlerEmbeddedShellRunsViaOverridableRunner(t *testing.T) {
	origRunner := runner
	defer func() { runner = origRunner }()
	var capturedExe string
	var capturedArgs []string
	var capturedScriptContent string
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		capturedExe = exe
		capturedArgs = args
		if len(args) > 0 {
			if data, err := os.ReadFile(args[len(args)-1]); err == nil {
				capturedScriptContent = string(data)
			}
		}
		return ironexec.Result{RC: 0, Stdout: "computed-value\n", StdoutLines: []string{"computed-value"}}, nil
	})

	h := factHandler{}
	item := map[string]any{"name": "computed", "shell": map[string]any{"command": "Write-Output computed-value"}}
	exec, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if capturedExe != "pwsh" {
		t.Fatalf("expected the default host to shell out to pwsh, got %q (args=%v)", capturedExe, capturedArgs)
	}
	if capturedScriptContent != "Write-Output computed-value" {
		t.Fatalf("expected the embedded command written to the temp script, got %q", capturedScriptContent)
	}
	if exec.Stdout != "computed-value\n" {
		t.Fatalf("exec.Stdout = %q", exec.Stdout)
	}
}

func TestFactHandlerDescribeRedactsSecretValue(t *testing.T) {
	h := factHandler{}
	item := map[string]any{"name": "$token", "value": "super-secret-value"}
	desc, err := h.Describe(item, engine.ActionInstall, engine.Context{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(desc, "super-secret-value") {
		t.Fatalf("Describe leaked secret value: %q", desc)
	}
	if !strings.Contains(desc, "***") {
		t.Fatalf("Describe should redact value in secret fact output: %q", desc)
	}
}

type fakeRunnerFunc func(exe string, args []string) (ironexec.Result, error)

func (f fakeRunnerFunc) Run(exe string, args []string) (ironexec.Result, error) { return f(exe, args) }

func TestZipHandlerCreatesGatesInstall(t *testing.T) {
	h := zipHandler{}
	dir := t.TempDir()
	marker := filepath.Join(dir, "installed.marker")
	item := map[string]any{"creates": []any{marker}}

	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before marker exists", installed, err)
	}
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true once marker exists", installed, err)
	}
}

func TestZipHandlerDownloadAndExtractUsesOverridableHTTP(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "src.zip")
	writeTestZip(t, zipPath, map[string]string{"file.txt": "payload"})

	origGet := httpGet
	defer func() { httpGet = origGet }()
	httpGet = func(url string) (*http.Response, error) {
		f, err := os.Open(zipPath) //nolint:gosec // zipPath is a t.TempDir()-derived path this same test just wrote, not user input
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: f}, nil
	}

	destDir := filepath.Join(dir, "dest")
	h := zipHandler{}
	item := map[string]any{"src": "http://example.invalid/src.zip", "dest": destDir}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "file.txt")) //nolint:gosec // destDir is a t.TempDir()-derived path this same test just wrote, not user input
	if err != nil || string(data) != "payload" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

func TestZipHandlerRejectsZipSlipEntry(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "malicious.zip")
	writeTestZip(t, zipPath, map[string]string{"../sneaky-file": "payload"})

	origGet := httpGet
	defer func() { httpGet = origGet }()
	httpGet = func(url string) (*http.Response, error) {
		f, err := os.Open(zipPath) //nolint:gosec // zipPath is a t.TempDir()-derived path this same test just wrote, not user input
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: f}, nil
	}

	destDir := filepath.Join(dir, "dest")
	h := zipHandler{}
	item := map[string]any{"src": "http://example.invalid/malicious.zip", "dest": destDir}
	if _, err := h.Install(item, "", testCtx()); err == nil {
		t.Fatal("expected an error for a zip entry containing '..', got nil")
	}
	if _, err := os.Stat(filepath.Join(dir, "sneaky-file")); err == nil {
		t.Fatal("zip-slip entry was written outside dest")
	}
}

// TestZipHandlerRejectsEntryNameSubstringDotDot documents a deliberate
// precision trade-off: the loop's literal 'strings.Contains(name, "..")'
// guard (added to match CodeQL's go/zipslip documented "GOOD" pattern -
// see zip.go) also rejects a benign filename that merely contains ".." as
// text, not just an actual '..' path-traversal segment. Accepted as the
// cost of a static-analysis-recognizable check; isSafeZipEntryName/
// safeExtractPath alone would have allowed it (see
// TestIsSafeZipEntryName/TestSafeExtractPathAllowsLegitFilenameContainingDotDot).
func TestZipHandlerRejectsEntryNameSubstringDotDot(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "benign.zip")
	writeTestZip(t, zipPath, map[string]string{"video..final.mp4": "payload"})

	origGet := httpGet
	defer func() { httpGet = origGet }()
	httpGet = func(url string) (*http.Response, error) {
		f, err := os.Open(zipPath) //nolint:gosec // zipPath is a t.TempDir()-derived path this same test just wrote, not user input
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Body: f}, nil
	}

	destDir := filepath.Join(dir, "dest")
	h := zipHandler{}
	item := map[string]any{"src": "http://example.invalid/benign.zip", "dest": destDir}
	if _, err := h.Install(item, "", testCtx()); err == nil {
		t.Fatal("expected the loop's stricter literal '..' substring guard to reject this entry name")
	}
}

func TestSafeExtractPathRejectsTraversalAndAbsolutePaths(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "dest")
	names := []string{
		"../sneaky-file",
		"a/../../sneaky-file",
		`..\sneaky-file`, // backslash-smuggled traversal on a zip's always-'/' entry name
		"/etc/passwd",
	}
	if runtime.GOOS == "windows" {
		names = append(names, `C:\Windows\System32\evil.dll`) // drive-letter absolute path - filepath.IsAbs is Windows-specific here
	}
	for _, name := range names {
		if _, err := safeExtractPath(dest, name); err == nil {
			t.Fatalf("safeExtractPath(%q) = nil error, want rejection", name)
		}
	}
}

func TestSafeExtractPathAllowsLegitFilenameContainingDotDot(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "dest")
	got, err := safeExtractPath(dest, "video..final.mp4")
	if err != nil {
		t.Fatalf("safeExtractPath rejected a legitimate filename containing \"..\": %v", err)
	}
	want, err := filepath.Abs(filepath.Join(dest, "video..final.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("safeExtractPath = %q, want %q", got, want)
	}
}

func TestIsSafeZipEntryName(t *testing.T) {
	unsafe := []string{"", "../sneaky-file", "a/../../sneaky-file", `..\sneaky-file`, "/etc/passwd"}
	if runtime.GOOS == "windows" {
		unsafe = append(unsafe, `C:\Windows\System32\evil.dll`)
	}
	for _, name := range unsafe {
		if isSafeZipEntryName(name) {
			t.Fatalf("isSafeZipEntryName(%q) = true, want false", name)
		}
	}

	for _, name := range []string{"file.txt", "sub/dir/file.txt", "video..final.mp4"} {
		if !isSafeZipEntryName(name) {
			t.Fatalf("isSafeZipEntryName(%q) = false, want true", name)
		}
	}
}

func writeTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path) //nolint:gosec // path is a t.TempDir()-derived path supplied by the calling test, not user input
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	zw := zip.NewWriter(f)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
}
