package handlers

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

func TestWindowsOwnerGroupPowerShellArguments(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not installed")
	}
	original := runner
	t.Cleanup(func() { runner = original })
	for _, tc := range []struct {
		name, path, owner, group string
	}{
		{"owner only", `C:\Users\admin\.editorconfig`, `MACHINE\admin`, ""},
		{"group only", `C:\space dir\[file].txt`, "", `BUILTIN\Users`},
		{"literal values", "C:\\space dir\\O'Brien\\$file; [x]\u2019.txt", "domain\\O'Brien", "group'$name;[x]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var output string
			runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
				if exe != "pwsh" || len(args) != 4 || args[2] != "-Command" {
					t.Fatalf("unexpected invocation: %s %q", exe, args)
				}
				prefix := `
function Get-Acl {
    param([string]$LiteralPath)
    $script:readPath = $LiteralPath
    $acl = [pscustomobject]@{}
    $acl | Add-Member ScriptMethod SetOwner { param($owner) $script:owner = $owner.Value }
    $acl | Add-Member ScriptMethod SetGroup { param($group) $script:group = $group.Value }
    return $acl
}
function Set-Acl {
    param([string]$LiteralPath, $AclObject)
    [pscustomobject]@{readPath=$script:readPath;writePath=$LiteralPath;owner=$script:owner;group=$script:group} | ConvertTo-Json -Compress
}
`
				result, err := ironexec.Default.Run(exe, append(args[:3:3], prefix+args[3]))
				output = result.Stdout
				return result, err
			})
			if err := applyWindowsOwnerGroup(tc.path, tc.owner, tc.group); err != nil {
				t.Fatal(err)
			}
			var got map[string]string
			if err := json.Unmarshal([]byte(output), &got); err != nil {
				t.Fatalf("invalid output %q: %v", output, err)
			}
			if got["readPath"] != tc.path || got["writePath"] != tc.path || got["owner"] != tc.owner || got["group"] != tc.group {
				t.Fatalf("PowerShell received %#v, want path=%q owner=%q group=%q", got, tc.path, tc.owner, tc.group)
			}
		})
	}
}

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
