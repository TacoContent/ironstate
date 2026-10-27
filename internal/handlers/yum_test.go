package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (yumHandler{}).ScanRole(); got != "roles/packages/yum" {
		t.Fatalf("ScanRole() = %q, want roles/packages/yum", got)
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
