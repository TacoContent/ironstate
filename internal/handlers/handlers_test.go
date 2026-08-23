package handlers

import (
	"archive/zip"
	"net/http"
	"os"
	"path/filepath"
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
