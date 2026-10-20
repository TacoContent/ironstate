package handlers

import (
	"errors"
	"os/exec"
	"strings"
	"testing"

	ironexec "github.com/TacoContent/ironstate/internal/exec"
)

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
