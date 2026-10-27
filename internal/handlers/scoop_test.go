package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (scoopHandler{}).ScanRole(); got != "roles/packages/scoop" {
		t.Fatalf("ScanRole() = %q, want roles/packages/scoop", got)
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

func TestScoopHandlerScanRolePath(t *testing.T) {
	scoop := scoopHandler{}
	if got := scoop.ScanRole(); got != "roles/packages/scoop" {
		t.Fatalf("ScanRole() = %q, want roles/packages/scoop", got)
	}
}
