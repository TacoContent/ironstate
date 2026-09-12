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

func TestPacmanHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "pacman" {
		t.Fatalf("exe = %q, want pacman", rec.exes[0])
	}
	want := []string{"-S", "--noconfirm", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestPacmanHandlerUpdateCacheThenInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}, {RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	item := map[string]any{"package": "git", "update_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "-Sy" || rec.calls[1][0] != "-S" {
		t.Fatalf("calls = %v", rec.calls)
	}
}

func TestPacmanHandlerInstallWithForce(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	item := map[string]any{"package": "git", "force": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"-S", "--noconfirm", "--overwrite=*", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestPacmanHandlerUninstallWithRecurseAndNosave(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	item := map[string]any{"package": "git", "recurse": true, "nosave": true}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"-Rsn", "--noconfirm", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestPacmanHandlerUpgradeOnlyTaskAlwaysRuns(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	item := map[string]any{"upgrade": true}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (so Install always runs)", installed, err)
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"-Syu", "--noconfirm"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestPacmanHandlerTestChecksPacmanQ(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := pacmanHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "pacman" || strings.Join(rec.calls[0], " ") != "-Q git" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestPacmanHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := pacmanHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingpkg"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestPacmanHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (pacmanHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestPacmanIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["pacman"].(pacmanHandler); !ok {
		t.Fatal("expected 'pacman' to be registered as pacmanHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "pacman" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'pacman' in AllModuleNames")
	}
}

func TestYumHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "yum" {
		t.Fatalf("exe = %q, want yum", rec.exes[0])
	}
	want := []string{"install", "-y", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestYumHandlerUpdateCacheThenInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}, {RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	item := map[string]any{"package": "git", "update_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "makecache" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
}

func TestYumHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	item := map[string]any{"package": "git", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "update" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestYumHandlerInstallWithRepoFlags(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	item := map[string]any{
		"package":           "git",
		"enablerepo":        "epel",
		"disablerepo":       "updates",
		"exclude":           "kernel*",
		"disable_gpg_check": true,
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "-y", "--enablerepo=epel", "--disablerepo=updates", "--exclude=kernel*", "--nogpgcheck", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestYumHandlerUninstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	item := map[string]any{"package": "git"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"remove", "-y", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestYumHandlerTestChecksRpmQuery(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := yumHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "rpm" || strings.Join(rec.calls[0], " ") != "-q git" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestYumHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := yumHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingpkg"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestYumHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (yumHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestYumIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["yum"].(yumHandler); !ok {
		t.Fatal("expected 'yum' to be registered as yumHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "yum" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'yum' in AllModuleNames")
	}
}

func TestApkHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "apk" {
		t.Fatalf("exe = %q, want apk", rec.exes[0])
	}
	want := []string{"add", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestApkHandlerUpdateCacheThenInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}, {RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"package": "git", "update_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "update" || rec.calls[1][0] != "add" {
		t.Fatalf("calls = %v", rec.calls)
	}
}

func TestApkHandlerLatestStateAddsUpgradeFlag(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"package": "git", "state": "latest"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "-u", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestApkHandlerInstallWithRepositoryAndNoCache(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"package": "git", "repository": "http://example.com/testing", "no_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "--repository", "http://example.com/testing", "--no-cache", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestApkHandlerUpgradeOnlyTaskAlwaysRuns(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"upgrade": true}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (so Install always runs)", installed, err)
	}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"upgrade"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestApkHandlerUninstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := apkHandler{}
	item := map[string]any{"package": "git"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"del", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestApkHandlerTestChecksApkInfo(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "git"}}}
	withRunner(t, rec)

	h := apkHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "apk" || strings.Join(rec.calls[0], " ") != "info -e git" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestApkHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0, Stdout: "git"},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := apkHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingpkg"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestApkHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (apkHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestApkWorldPackageNameStripsConstraints(t *testing.T) {
	cases := map[string]string{
		"git":          "git",
		"git=1.2.3-r0": "git",
		"git>=1.0":     "git",
		"git<2.0":      "git",
		"git~1.0":      "git",
		"git@testing":  "git",
		"git%provider": "git",
		"  git  ":      "git",
	}
	for in, want := range cases {
		if got := apkWorldPackageName(in); got != want {
			t.Errorf("apkWorldPackageName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestApkIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["apk"].(apkHandler); !ok {
		t.Fatal("expected 'apk' to be registered as apkHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "apk" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'apk' in AllModuleNames")
	}
}

func TestSnapHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := snapHandler{}
	item := map[string]any{"package": []any{"hello", "code"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "snap" {
		t.Fatalf("exe = %q, want snap", rec.exes[0])
	}
	want := []string{"install", "hello", "code"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestSnapHandlerInstallWithClassicAndChannel(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := snapHandler{}
	item := map[string]any{"package": "code", "classic": true, "channel": "edge"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "--classic", "--channel=edge", "code"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestSnapHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := snapHandler{}
	item := map[string]any{"package": "hello", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "refresh" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestSnapHandlerUninstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := snapHandler{}
	item := map[string]any{"package": "hello"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"remove", "hello"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestSnapHandlerTestChecksSnapList(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "Name  Version  Rev  Tracking  Publisher  Notes\nhello  2.10  38  latest/stable  canonical  -"}}}
	withRunner(t, rec)

	h := snapHandler{}
	installed, err := h.Test(map[string]any{"package": "hello"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "snap" || strings.Join(rec.calls[0], " ") != "list hello" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestSnapHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := snapHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"hello", "missingsnap"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (hello is present, so uninstall should run)", installed, err)
	}
}

func TestSnapHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (snapHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestSnapIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["snap"].(snapHandler); !ok {
		t.Fatal("expected 'snap' to be registered as snapHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "snap" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'snap' in AllModuleNames")
	}
}

func TestFlatpakHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := flatpakHandler{}
	item := map[string]any{"package": []any{"org.videolan.VLC", "org.gimp.GIMP"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "flatpak" {
		t.Fatalf("exe = %q, want flatpak", rec.exes[0])
	}
	want := []string{"install", "-y", "--noninteractive", "flathub", "org.videolan.VLC", "org.gimp.GIMP"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestFlatpakHandlerInstallWithRemoteAndUserScope(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := flatpakHandler{}
	item := map[string]any{"package": "org.gimp.GIMP", "remote": "myremote", "method": "user"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "-y", "--noninteractive", "--user", "myremote", "org.gimp.GIMP"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestFlatpakHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := flatpakHandler{}
	item := map[string]any{"package": "org.gimp.GIMP", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "update" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestFlatpakHandlerUninstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := flatpakHandler{}
	item := map[string]any{"package": "org.gimp.GIMP"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"uninstall", "-y", "--noninteractive", "org.gimp.GIMP"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestFlatpakHandlerTestChecksFlatpakInfo(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := flatpakHandler{}
	installed, err := h.Test(map[string]any{"package": "org.gimp.GIMP"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "flatpak" || strings.Join(rec.calls[0], " ") != "info org.gimp.GIMP" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestFlatpakHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0},
		{RC: 1},
	}}
	withRunner(t, rec)

	h := flatpakHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"org.gimp.GIMP", "missing.app"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (org.gimp.GIMP is present, so uninstall should run)", installed, err)
	}
}

func TestFlatpakHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (flatpakHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestFlatpakIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["flatpak"].(flatpakHandler); !ok {
		t.Fatal("expected 'flatpak' to be registered as flatpakHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "flatpak" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'flatpak' in AllModuleNames")
	}
}

func TestScoopHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := scoopHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "scoop" {
		t.Fatalf("exe = %q, want scoop", rec.exes[0])
	}
	want := []string{"install", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestScoopHandlerInstallWithGlobalAndArchitecture(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := scoopHandler{}
	item := map[string]any{"package": "git", "global": true, "architecture": "64bit"}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"install", "-g", "--arch=64bit", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestScoopHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := scoopHandler{}
	item := map[string]any{"package": "git", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "update" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestScoopHandlerUninstallWithGlobal(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := scoopHandler{}
	item := map[string]any{"package": "git", "global": true}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"uninstall", "-g", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestScoopHandlerTestChecksScoopList(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "Name  Version  Source  Updated  Info\n----  -------  ------  -------  ----\ngit   2.40.0   main    today    "}}}
	withRunner(t, rec)

	h := scoopHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "scoop" || strings.Join(rec.calls[0], " ") != "list git" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestScoopHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0, Stdout: "git 2.40.0"},
		{RC: 0, Stdout: ""},
	}}
	withRunner(t, rec)

	h := scoopHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingpkg"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestScoopHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (scoopHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestScoopIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["scoop"].(scoopHandler); !ok {
		t.Fatal("expected 'scoop' to be registered as scoopHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "scoop" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'scoop' in AllModuleNames")
	}
}

func TestMacportsHandlerInstallBatchesMultiplePackages(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := macportsHandler{}
	item := map[string]any{"package": []any{"git", "curl"}}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if rec.exes[0] != "port" {
		t.Fatalf("exe = %q, want port", rec.exes[0])
	}
	want := []string{"install", "git", "curl"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestMacportsHandlerUpdateCacheThenInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}, {RC: 0}}}
	withRunner(t, rec)

	h := macportsHandler{}
	item := map[string]any{"package": "git", "update_cache": true}
	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.calls[0][0] != "selfupdate" || rec.calls[1][0] != "install" {
		t.Fatalf("calls = %v", rec.calls)
	}
}

func TestMacportsHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := macportsHandler{}
	item := map[string]any{"package": "git", "state": "latest"}
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

func TestMacportsHandlerUninstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0}}}
	withRunner(t, rec)

	h := macportsHandler{}
	item := map[string]any{"package": "git"}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	want := []string{"uninstall", "git"}
	if strings.Join(rec.calls[0], " ") != strings.Join(want, " ") {
		t.Fatalf("args = %v, want %v", rec.calls[0], want)
	}
}

func TestMacportsHandlerTestChecksPortInstalled(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "  git @2.40.0_0 (active)"}}}
	withRunner(t, rec)

	h := macportsHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true", installed, err)
	}
	if rec.exes[0] != "port" || strings.Join(rec.calls[0], " ") != "-q installed git" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestMacportsHandlerTestNotInstalled(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 0, Stdout: "None of the specified ports are installed."}}}
	withRunner(t, rec)

	h := macportsHandler{}
	installed, err := h.Test(map[string]any{"package": "git"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
}

func TestMacportsHandlerTestAbsentReturnsTrueIfAnyPackagePresent(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{
		{RC: 0, Stdout: "  git @2.40.0_0 (active)"},
		{RC: 0, Stdout: "None of the specified ports are installed."},
	}}
	withRunner(t, rec)

	h := macportsHandler{}
	installed, err := h.Test(map[string]any{"package": []any{"git", "missingport"}, "state": "absent"}, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true (git is present, so uninstall should run)", installed, err)
	}
}

func TestMacportsHandlerScanRoleIsRolesPackages(t *testing.T) {
	if got := (macportsHandler{}).ScanRole(); got != "roles/packages" {
		t.Fatalf("ScanRole() = %q, want roles/packages", got)
	}
}

func TestMacportsIsRegistered(t *testing.T) {
	all := All()
	if _, ok := all["macports"].(macportsHandler); !ok {
		t.Fatal("expected 'macports' to be registered as macportsHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "macports" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'macports' in AllModuleNames")
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

func TestXgetHandlerResolvesToArgAndTests(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/delta.exe"
	item := map[string]any{"package": "dandavison/delta", "args": []any{"--to=" + target, "--upgrade-only"}}

	h := xgetHandler{}
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
