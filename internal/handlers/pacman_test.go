package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (pacmanHandler{}).ScanRole(); got != "roles/packages/pacman" {
		t.Fatalf("ScanRole() = %q, want roles/packages/pacman", got)
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

func TestPacmanHandlerScanRolePath(t *testing.T) {
	pacman := pacmanHandler{}
	if got := pacman.ScanRole(); got != "roles/packages/pacman" {
		t.Fatalf("ScanRole() = %q, want roles/packages/pacman", got)
	}
}
