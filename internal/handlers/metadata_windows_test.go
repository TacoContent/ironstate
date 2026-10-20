package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestWindowsOwnerGroupRealACL(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not installed")
	}
	target := filepath.Join(t.TempDir(), "owned O'Brien [literal].txt")
	if err := os.WriteFile(target, []byte("content"), 0o600); err != nil {
		t.Fatal(err)
	}
	readACL := func() map[string]string {
		t.Helper()
		script := "$ErrorActionPreference='Stop';$acl=Get-Acl -LiteralPath " + pwshSingleQuote(target) + ";@{owner=$acl.Owner;group=$acl.Group;access=$acl.AccessToString;protected=[string]$acl.AreAccessRulesProtected}|ConvertTo-Json -Compress"
		res, err := ironexec.Default.Run("pwsh", []string{"-NoProfile", "-NonInteractive", "-Command", script})
		if err != nil || res.RC != 0 {
			t.Fatalf("read ACL: %#v, %v", res, err)
		}
		var acl map[string]string
		if err := json.Unmarshal([]byte(res.Stdout), &acl); err != nil {
			t.Fatal(err)
		}
		return acl
	}
	before := readACL()
	original := runner
	runner = ironexec.Default
	t.Cleanup(func() { runner = original })
	if err := applyWindowsOwnerGroup(target, before["owner"], ""); err != nil {
		t.Fatal(err)
	}
	after := readACL()
	for key, value := range before {
		if after[key] != value {
			t.Fatalf("ACL %s unexpectedly changed: before=%#v after=%#v", key, before, after)
		}
	}
}
