package handlers

import (
	"os"
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

// recordingRunner captures every Run() call's argv and returns a queued
// canned response per call (or the last one, once the queue is drained).
type recordingRunner struct {
	calls     [][]string
	exes      []string
	responses []ironexec.Result
	errs      []error
}

func (r *recordingRunner) Run(exe string, args []string) (ironexec.Result, error) {
	r.exes = append(r.exes, exe)
	r.calls = append(r.calls, args)
	idx := len(r.calls) - 1
	var result ironexec.Result
	var err error
	if idx < len(r.responses) {
		result = r.responses[idx]
	} else if len(r.responses) > 0 {
		result = r.responses[len(r.responses)-1]
	}
	if idx < len(r.errs) {
		err = r.errs[idx]
	}
	return result, err
}

func withRunner(t *testing.T, r *recordingRunner) {
	t.Helper()
	orig := runner
	runner = r
	t.Cleanup(func() { runner = orig })
}

func TestWingetHandlerInstallArgv(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := wingetHandler{}
	item := map[string]any{"package": "FiloSottile.age", "source": "winget"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "winget" {
		t.Fatalf("exe = %q", rec.exes[0])
	}
	want := []string{"install", "--id", "FiloSottile.age", "--exact", "--accept-source-agreements", "--accept-package-agreements", "--source", "winget"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestWingetHandlerTestChecksOutputText(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "No installed package found matching input criteria."}}}
	withRunner(t, rec)

	h := wingetHandler{}
	installed, err := h.Test(map[string]any{"package": "x"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
}

func TestChocolateyHandlerLatestStateUpgrades(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := chocolateyHandler{}
	item := map[string]any{"package": "ripgrep", "state": "latest"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "choco" || rec.calls[0][0] != "upgrade" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestPipxHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := pipxHandler{}
	item := map[string]any{"package": "black", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "upgrade" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestHomebrewHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := homebrewHandler{}
	item := map[string]any{"package": "ripgrep", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.exes[0] != "brew" || rec.calls[0][0] != "upgrade" || rec.calls[1][0] != "install" {
		t.Fatalf("exes=%v calls=%v", rec.exes, rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestHomebrewHandlerTestChecksBrewList(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1, Stderr: "Error: No such keg"}}}
	withRunner(t, rec)

	h := homebrewHandler{}
	installed, err := h.Test(map[string]any{"package": "ripgrep"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
	if rec.exes[0] != "brew" || strings.Join(rec.calls[0], " ") != "list ripgrep" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestBrewIsRegisteredAsHomebrewAlias(t *testing.T) {
	all := All()
	if _, ok := all["brew"].(homebrewHandler); !ok {
		t.Fatal("expected 'brew' to be registered as homebrewHandler")
	}
	if _, ok := all["homebrew"].(homebrewHandler); !ok {
		t.Fatal("expected 'homebrew' to be registered as homebrewHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "brew" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'brew' in AllModuleNames")
	}
}

func TestAptHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := aptHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "apt-get" {
		t.Fatalf("exe = %q, want apt-get", rec.exes[0])
	}
	want := []string{"install", "-y", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestAptHandlerUpdateCacheThenInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}, {RC: 0}}}
	withRunner(t, rec)

	h := aptHandler{}
	item := map[string]any{"package": "git", "update_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "update" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
}

func TestAptHandlerUninstallWithPurge(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := aptHandler{}
	item := map[string]any{"package": "git", "purge": true}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"purge", "-y", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestAptHandlerAutoremoveOnlyTaskAlwaysRuns(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := aptHandler{}
	item := map[string]any{"autoremove": true}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (so Install always runs)", installed, err)
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"autoremove", "-y"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestAptHandlerTestChecksDpkgQuery(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "install ok installed"}}}
	withRunner(t, rec)

	h := aptHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "dpkg-query" {
		t.Fatalf("exe = %q, want dpkg-query", rec.exes[0])
	}
}

func TestAptHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0, Stdout: "install ok installed"},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := aptHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingpkg"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestEgetHandlerResolvesToArgAndTests(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/delta.exe"
	item := map[string]any{"package": "dandavison/delta", "args": []any{"--to=" + target, "--upgrade-only"}}

	h := egetHandler{}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before target exists", installed, err)
	}

	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true once target exists", installed, err)
	}
}

func TestGoHandlerBinaryPathAndUninstall(t *testing.T) {
	dir := t.TempDir()
	goBinDirCache = dir
	defer func() { goBinDirCache = "" }()

	item := map[string]any{"package": "github.com/x/tool"}
	path := goBinaryPath(item)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := goHandler{}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if fileExists(path) {
		t.Fatal("expected binary removed")
	}
}

func TestShellHandlerRunsCommandViaPwshByDefault(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "hi\n", StdoutLines: []string{"hi"}}}}
	withRunner(t, rec)

	h := shellHandler{}
	item := map[string]any{"command": "Write-Output hi"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "pwsh" {
		t.Fatalf("exe = %q, want pwsh", rec.exes[0])
	}
	if result.Stdout != "hi\n" {
		t.Fatalf("result = %#v", result)
	}
}

func TestShellHandlerPerStateFallback(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := shellHandler{}
	item := map[string]any{
		"host":    "cmd",
		"command": "echo top-level",
		"present": map[string]any{"command": "echo present-state"},
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	// per-state 'present' block's own 'command' wins, falls back to
	// top-level 'host' since the block doesn't set its own.
	if rec.exes[0] != "cmd.exe" {
		t.Fatalf("exe = %q, want cmd.exe (host falls back to top-level)", rec.exes[0])
	}
}

func TestShellHandlerAbsentDoesNotFallBackToTopLevelCommand(t *testing.T) {
	cfg := resolveShellStateConfig(map[string]any{"command": "echo install-only"}, "absent")
	if cfg.Command != "" {
		t.Fatalf("expected 'absent' to never fall back to the top-level command, got %q", cfg.Command)
	}
}

func TestWingetPackagesToEntriesKeepsIdsAndSources(t *testing.T) {
	previousLookup := wingetDisplayNameLookup
	wingetDisplayNameLookup = func(identifier string) string {
		if identifier == "9P8LTPGCBZXD" {
			return "Wintoys"
		}
		return ""
	}
	defer func() {
		wingetDisplayNameLookup = previousLookup
	}()

	items := wingetPackagesToEntries([]wingetExportPackage{
		{Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Identifier: "7zip.7zip", Source: "winget"},
	})
	if len(items) != 4 {
		t.Fatalf("items = %d, want 4", len(items))
	}
	for _, want := range []wingetPackage{
		{Name: "Microsoft.VCRedist.2010.x64", Identifier: "Microsoft.VCRedist.2010.x64", Source: "winget"},
		{Name: "Microsoft.VCRedist.2010.x86", Identifier: "Microsoft.VCRedist.2010.x86", Source: "winget"},
		{Name: "Wintoys", Identifier: "9P8LTPGCBZXD", Source: "msstore"},
		{Name: "7zip.7zip", Identifier: "7zip.7zip", Source: "winget"},
	} {
		matched := false
		for _, got := range items {
			if got.Identifier == want.Identifier {
				matched = true
				if got != want {
					t.Fatalf("package %+v, want %+v", got, want)
				}
			}
		}
		if !matched {
			t.Fatalf("missing package identifier %q", want.Identifier)
		}
	}
}

func TestWingetHandlerScanRoleAndRoleAreRolesPackages(t *testing.T) {
	winget := wingetHandler{}
	if got := winget.ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
	choco := chocolateyHandler{}
	if got := choco.ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
	brew := homebrewHandler{}
	if got := brew.ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
	apt := aptHandler{}
	if got := apt.ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
	npm := npmHandler{}
	if got := npm.ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestBrewListToItemsSkipsBlankLines(t *testing.T) {
	items := brewListToItems("ripgrep\nfd\n\nbat\n")
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for _, want := range []string{"ripgrep", "fd", "bat"} {
		matched := false
		for _, got := range items {
			if got.Name == want && got.Module == "homebrew" && got.Config["package"] == want {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("missing entry %q in %+v", want, items)
		}
	}
}

func TestShellHandlerCreatesGatesTest(t *testing.T) {
	dir := t.TempDir()
	marker := dir + "/done.marker"
	h := shellHandler{}
	item := map[string]any{"creates": []any{marker}}

	installed, _ := h.Test(item, "", testCtx())
	if installed {
		t.Fatal("expected not installed before marker exists")
	}
	if err := os.WriteFile(marker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, _ = h.Test(item, "", testCtx())
	if !installed {
		t.Fatal("expected installed once marker exists")
	}
}
