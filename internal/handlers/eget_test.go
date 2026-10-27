package handlers

import (
	"os"
	"testing"
)

func TestEgetHandlerResolvesToArgAndTests(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/delta.exe"
	item := map[string]any{"package": "dandavison/delta", "args": []any{"--to=" + target, "--upgrade-only"}}

	h := egetHandler{}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || installed {
		t.Fatalf("installed=%v err=%v, want false before target exists", installed, err)
	}

	if err := os.WriteFile(target, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	installed, err = h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v, want true once target exists", installed, err)
	}
}
