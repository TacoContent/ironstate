package handlers

import (
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
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

func TestWindowsOwnerGroupReportsErrors(t *testing.T) {
	original := runner
	t.Cleanup(func() { runner = original })
	launchErr := errors.New("pwsh unavailable")
	runner = fakeRunnerFunc(func(string, []string) (ironexec.Result, error) {
		return ironexec.Result{}, launchErr
	})
	if err := applyWindowsOwnerGroup("target", "owner", ""); !errors.Is(err, launchErr) || !strings.Contains(err.Error(), "target") {
		t.Fatalf("launch error = %v", err)
	}
	for _, stderr := range []string{"access denied", ""} {
		runner = fakeRunnerFunc(func(string, []string) (ironexec.Result, error) {
			return ironexec.Result{RC: 1, Stderr: stderr}, nil
		})
		err := applyWindowsOwnerGroup("target", "owner", "")
		if err == nil || !strings.Contains(err.Error(), "target") || (stderr != "" && !strings.Contains(err.Error(), stderr)) {
			t.Fatalf("ACL error = %v", err)
		}
	}
}

func TestWindowsOwnerGroupStopsOnACLReadError(t *testing.T) {
	if _, err := exec.LookPath("pwsh"); err != nil {
		t.Skip("pwsh is not installed")
	}
	original := runner
	t.Cleanup(func() { runner = original })
	runner = fakeRunnerFunc(func(exe string, args []string) (ironexec.Result, error) {
		prefix := "function Get-Acl { Write-Error 'ACL read failed' }; function Set-Acl { throw 'unexpected ACL write' };"
		return ironexec.Default.Run(exe, append(args[:3:3], prefix+args[3]))
	})
	err := applyWindowsOwnerGroup("target", "owner", "")
	if err == nil || !strings.Contains(err.Error(), "ACL read failed") || strings.Contains(err.Error(), "unexpected ACL write") {
		t.Fatalf("error = %v, want original read error without attempting a write", err)
	}
}
