package handlers

import (
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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

func TestPipxHandlerScanRolePath(t *testing.T) {
	pipx := pipxHandler{}
	if got := pipx.ScanRole(); got != "roles/packages/pipx" {
		t.Fatalf("ScanRole() = %q, want roles/packages/pipx", got)
	}
}
