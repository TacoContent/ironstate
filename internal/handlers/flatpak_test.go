package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
	if got := (flatpakHandler{}).ScanRole(); got != "roles/packages/flatpak" {
		t.Fatalf("ScanRole() = %q, want roles/packages/flatpak", got)
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

func TestFlatpakHandlerScanRolePath(t *testing.T) {
	flatpak := flatpakHandler{}
	if got := flatpak.ScanRole(); got != "roles/packages/flatpak" {
		t.Fatalf("ScanRole() = %q, want roles/packages/flatpak", got)
	}
}
