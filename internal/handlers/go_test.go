package handlers

import (
	"os"
	"testing"
)

func TestGoHandlerBinaryPathAndUninstall(t *testing.T) {
	dir := t.TempDir()
	goBinDirCache = dir
	defer func() { goBinDirCache = "" }()

	item := map[string]any{"package": "github.com/x/tool"}
	path := goBinaryPath(item)
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	h := goHandler{}
	installed, err := h.Test(item, "", testCtx())
	if err != nil || !installed {
		t.Fatalf("installed=%v err=%v", installed, err)
	}
	if _, err := h.Uninstall(item, "", testCtx()); err != nil {
		t.Fatal(err)
	}
	if fileExists(path) {
		t.Fatal("expected binary removed")
	}
}
