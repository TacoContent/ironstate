package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
