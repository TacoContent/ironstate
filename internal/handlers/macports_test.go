package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (macportsHandler{}).ScanRole(); got != "roles/packages/macports" {
		t.Fatalf("ScanRole() = %q, want roles/packages/macports", got)
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

func TestMacportsHandlerScanRolePath(t *testing.T) {
	macports := macportsHandler{}
	if got := macports.ScanRole(); got != "roles/packages/macports" {
		t.Fatalf("ScanRole() = %q, want roles/packages/macports", got)
	}
}
