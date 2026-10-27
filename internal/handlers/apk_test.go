package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (apkHandler{}).ScanRole(); got != "roles/packages/apk" {
		t.Fatalf("ScanRole() = %q, want roles/packages/apk", got)
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

func TestApkHandlerScanRolePath(t *testing.T) {
	apk := apkHandler{}
	if got := apk.ScanRole(); got != "roles/packages/apk" {
		t.Fatalf("ScanRole() = %q, want roles/packages/apk", got)
	}
}
