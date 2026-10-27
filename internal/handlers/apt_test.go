package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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

func TestAptHandlerScanRolePath(t *testing.T) {
	apt := aptHandler{}
	if got := apt.ScanRole(); got != "roles/packages/apt" {
		t.Fatalf("ScanRole() = %q, want roles/packages/apt", got)
	}
}
