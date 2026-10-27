package handlers

import (
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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

func TestChocolateyHandlerScanRolePath(t *testing.T) {
	choco := chocolateyHandler{}
	if got := choco.ScanRole(); got != "roles/packages/chocolatey" {
		t.Fatalf("ScanRole() = %q, want roles/packages/chocolatey", got)
	}
}
