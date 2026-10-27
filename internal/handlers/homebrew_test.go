package handlers

import (
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestHomebrewHandlerLatestFallsBackToInstall(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1}, {RC: 0}}}
	withRunner(t, rec)

	h := homebrewHandler{}
	item := map[string]any{"package": "ripgrep", "state": "latest"}
	result, err := h.Install(item, "", testCtx())
	if err != nil {
		t.Fatal(err)
	}
	if len(rec.calls) != 2 || rec.exes[0] != "brew" || rec.calls[0][0] != "upgrade" || rec.calls[1][0] != "install" {
		t.Fatalf("exes=%v calls=%v", rec.exes, rec.calls)
	}
	if result.RC != 0 {
		t.Fatalf("result = %#v", result)
	}
}

func TestHomebrewHandlerTestChecksBrewList(t *testing.T) {
	rec := &recordingRunner{responses: []ironexec.Result{{RC: 1, Stderr: "Error: No such keg"}}}
	withRunner(t, rec)

	h := homebrewHandler{}
	installed, err := h.Test(map[string]any{"package": "ripgrep"}, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false", installed, err)
	}
	if rec.exes[0] != "brew" || strings.Join(rec.calls[0], " ") != "list ripgrep" {
		t.Fatalf("exe=%q args=%v", rec.exes[0], rec.calls[0])
	}
}

func TestBrewIsRegisteredAsHomebrewAlias(t *testing.T) {
	all := All()
	if _, ok := all["brew"].(homebrewHandler); !ok {
		t.Fatal("expected 'brew' to be registered as homebrewHandler")
	}
	if _, ok := all["homebrew"].(homebrewHandler); !ok {
		t.Fatal("expected 'homebrew' to be registered as homebrewHandler")
	}
	found := false
	for _, name := range AllModuleNames {
		if name == "brew" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected 'brew' in AllModuleNames")
	}
}

func TestBrewListToItemsSkipsBlankLines(t *testing.T) {
	items := brewListToItems("ripgrep\nfd\n\nbat\n")
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3", len(items))
	}
	for _, want := range []string{"ripgrep", "fd", "bat"} {
		matched := false
		for _, got := range items {
			if got.Name == want && got.Module == "homebrew" && got.Config["package"] == want {
				matched = true
			}
		}
		if !matched {
			t.Fatalf("missing entry %q in %+v", want, items)
		}
	}
}

func TestHomebrewHandlerScanRolePath(t *testing.T) {
	brew := homebrewHandler{}
	if got := brew.ScanRole(); got != "roles/packages/homebrew" {
		t.Fatalf("ScanRole() = %q, want roles/packages/homebrew", got)
	}
}
