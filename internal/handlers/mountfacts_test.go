package handlers

import (
	"testing"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/facts"
)

func TestMountFactsHandlerTestReflectsState(t *testing.T) {
	h := mountFactsHandler{}
	if installed, err := h.Test(map[string]any{}, "", testCtx()); err != nil || installed {
		t.Fatalf("Test(default state) = %v, %v; want false (always (re)gather)", installed, err)
	}
	if installed, err := h.Test(map[string]any{"state": "present"}, "", testCtx()); err != nil || installed {
		t.Fatalf("Test(present) = %v, %v; want false", installed, err)
	}
	if installed, err := h.Test(map[string]any{"state": "absent"}, "", testCtx()); err != nil || !installed {
		t.Fatalf("Test(absent) = %v, %v; want true (so it resolves to Uninstall)", installed, err)
	}
}

func TestMountFactsHandlerFactNameDefaultsToMounts(t *testing.T) {
	h := mountFactsHandler{}
	if name, ok := h.FactName(map[string]any{}); !ok || name != "mounts" {
		t.Fatalf("FactName(no name) = %q, %v; want \"mounts\", true", name, ok)
	}
	if name, ok := h.FactName(map[string]any{"name": "disks"}); !ok || name != "disks" {
		t.Fatalf("FactName(name=disks) = %q, %v; want \"disks\", true", name, ok)
	}
}

func TestMountFactsHandlerDescribe(t *testing.T) {
	h := mountFactsHandler{}
	desc, err := h.Describe(map[string]any{}, engine.ActionInstall, testCtx())
	if err != nil || desc != "gather mount facts -> fact 'mounts'" {
		t.Fatalf("Describe(install) = %q, %v", desc, err)
	}
	desc, err = h.Describe(map[string]any{"name": "disks"}, engine.ActionUninstall, testCtx())
	if err != nil || desc != "unset fact 'disks'" {
		t.Fatalf("Describe(uninstall) = %q, %v", desc, err)
	}
}

func TestMountFactsHandlerUninstallIsNoop(t *testing.T) {
	h := mountFactsHandler{}
	exec, err := h.Uninstall(map[string]any{}, "", testCtx())
	if err != nil || exec.RC != 0 {
		t.Fatalf("Uninstall() = %#v, %v; want a zero-value success result", exec, err)
	}
	if _, hasValue := exec.Extra["value"]; hasValue {
		t.Fatalf("Uninstall() Extra = %#v, want no 'value' (nothing to unset beyond deleting the fact itself)", exec.Extra)
	}
}

// TestMountFactsHandlerInstallGathersRealMounts exercises Install against
// this machine's real mounts (internal/facts' platform-specific gathering
// is already unit-tested in isolation via its own mocked platformMounts) -
// this is the "does the wiring actually produce a fact-shaped value" check
// at the handler layer.
func TestMountFactsHandlerInstallGathersRealMounts(t *testing.T) {
	h := mountFactsHandler{}
	exec, err := h.Install(map[string]any{}, "", testCtx())
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if exec.RC != 0 {
		t.Fatalf("Install() RC = %d, Stderr = %q", exec.RC, exec.Stderr)
	}
	value, ok := exec.Extra["value"].([]any)
	if !ok {
		t.Fatalf("Install() Extra[\"value\"] = %#v (%T), want []any", exec.Extra["value"], exec.Extra["value"])
	}
	if len(value) == 0 {
		t.Fatal("Install() gathered zero mounts, want at least one on any real host")
	}
	entry, ok := value[0].(map[string]any)
	if !ok {
		t.Fatalf("value[0] = %#v (%T), want map[string]any", value[0], value[0])
	}
	for _, key := range []string{"source", "device", "fstype", "options", "path"} {
		if _, present := entry[key]; !present {
			t.Errorf("mount entry missing key %q: %#v", key, entry)
		}
	}
}

func TestFilterMountsNoFilterKeepsEverything(t *testing.T) {
	mounts := []facts.MountFact{
		{Source: "/proc/mounts", Device: "/dev/sda1", FSType: "ext4", Options: "rw", Path: "/"},
		{Source: "/proc/mounts", Device: "none", FSType: "proc", Options: "rw", Path: "/proc"},
	}
	got, err := filterMounts(mounts, nil, nil)
	if err != nil {
		t.Fatalf("filterMounts() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filterMounts(no filter) = %d entries, want 2: %#v", len(got), got)
	}
}

func TestFilterMountsANDsMultipleConditions(t *testing.T) {
	mounts := []facts.MountFact{
		{Source: "GetVolumeInformation", Device: "\\\\?\\Volume{a}\\", FSType: "NTFS", Options: "rw", Path: "C:\\"},
		{Source: "WNetGetConnection", Device: "\\\\server\\share", FSType: "NTFS", Options: "rw", Path: "Z:\\"},
		{Source: "GetVolumeInformation", Device: "none", FSType: "proc", Options: "rw", Path: "/proc"},
	}
	filter := []any{
		`device not in ["none", "drivers"]`,
		`fstype == "NTFS"`,
		`source not in ["WNetGetConnection"]`,
	}
	got, err := filterMounts(mounts, filter, nil)
	if err != nil {
		t.Fatalf("filterMounts() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("filterMounts(3 AND'd conditions) = %d entries, want 1: %#v", len(got), got)
	}
	entry := got[0].(map[string]any)
	if entry["path"] != "C:\\" {
		t.Fatalf("filterMounts() kept %#v, want the C:\\ entry", entry)
	}
}

func TestFilterMountsPropagatesConditionError(t *testing.T) {
	mounts := []facts.MountFact{{Device: "/dev/sda1"}}
	if _, err := filterMounts(mounts, []any{"device ==="}, nil); err == nil {
		t.Fatal("filterMounts(invalid expression) = nil error, want an error")
	}
}

func TestMountFactsTimeoutParsesSecondsWithDefault(t *testing.T) {
	if got := mountFactsTimeout(map[string]any{}); got.Seconds() != 10 {
		t.Fatalf("mountFactsTimeout(default) = %v, want 10s", got)
	}
	if got := mountFactsTimeout(map[string]any{"timeout": 2.5}); got.Seconds() != 2.5 {
		t.Fatalf("mountFactsTimeout(2.5) = %v, want 2.5s", got)
	}
	if got := mountFactsTimeout(map[string]any{"timeout": 0.0}); got != 0 {
		t.Fatalf("mountFactsTimeout(0) = %v, want 0 (no bound)", got)
	}
}
