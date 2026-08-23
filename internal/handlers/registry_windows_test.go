//go:build windows

package handlers

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestRegistryHandlerSetTestRemove(t *testing.T) {
	testPath := `HKCU\Software\IronstateGoRewriteTest`
	t.Cleanup(func() {
		_ = registry.DeleteKey(registry.CURRENT_USER, `Software\IronstateGoRewriteTest`)
	})

	h := registryHandler{}
	item := map[string]any{
		"path": testPath,
		"values": []any{
			map[string]any{"name": "StringVal", "type": "String", "value": "hello"},
			map[string]any{"name": "DWordVal", "type": "DWord", "value": float64(42)},
		},
	}

	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before key exists", installed, err)
	}

	if _, err := h.Install(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true after install", installed, err)
	}

	// Drift: a differing value must be detected and corrected.
	item["values"] = []any{
		map[string]any{"name": "StringVal", "type": "String", "value": "changed"},
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false (drift detected)", installed, err)
	}

	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	item["state"] = "absent"
	installed, err = h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false after uninstall", installed, err)
	}
}
