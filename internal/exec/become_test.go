package exec

import (
	"errors"
	"strings"
	"testing"
)

func withSudoPath(t *testing.T, path string, err error) {
	t.Helper()
	orig := LookSudoPath
	LookSudoPath = func() (string, error) { return path, err }
	t.Cleanup(func() { LookSudoPath = orig })
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("args = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("args = %v, want %v", got, want)
		}
	}
}

func TestWrapForBecomeDisabledPassesThrough(t *testing.T) {
	exe, args, err := WrapForBecome(Become{}, "apt-get", []string{"install", "-y", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if exe != "apt-get" {
		t.Fatalf("exe = %q, want apt-get", exe)
	}
	assertArgs(t, args, []string{"install", "-y", "pkg"})
}

func TestWrapForBecomeEnabledPrependsSudo(t *testing.T) {
	withSudoPath(t, "/usr/bin/sudo", nil)

	exe, args, err := WrapForBecome(Become{Enabled: true}, "apt-get", []string{"install", "-y", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if exe != "/usr/bin/sudo" {
		t.Fatalf("exe = %q, want /usr/bin/sudo", exe)
	}
	assertArgs(t, args, []string{"apt-get", "install", "-y", "pkg"})
}

func TestWrapForBecomePasswordUsesNonInteractiveSudo(t *testing.T) {
	withSudoPath(t, "/usr/bin/sudo", nil)
	SetBecomePassword("example-password")
	t.Cleanup(func() { SetBecomePassword("") })

	exe, args, err := WrapForBecome(Become{Enabled: true}, "apt-get", []string{"install", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if exe != "/usr/bin/sudo" {
		t.Fatalf("exe = %q", exe)
	}
	assertArgs(t, args, []string{"-S", "-k", "-p", "", "apt-get", "install", "pkg"})
}

func TestRemoteWindowsBecomeRequiresElevatedSession(t *testing.T) {
	SetRemoteWindowsMode(true, false)
	t.Cleanup(func() { SetRemoteWindowsMode(false, false) })
	if err := ValidateBecome(Become{Enabled: true}); err == nil || !strings.Contains(err.Error(), "SSH session is not elevated") {
		t.Fatalf("ValidateBecome error = %v", err)
	}

	SetRemoteWindowsMode(true, true)
	exe, args, err := WrapForBecome(Become{Enabled: true}, "winget", []string{"install", "tool"})
	if err != nil {
		t.Fatal(err)
	}
	if exe != "winget" {
		t.Fatalf("exe = %q, want original command", exe)
	}
	assertArgs(t, args, []string{"install", "tool"})
}

func TestWrapForBecomeWithUserAddsDashU(t *testing.T) {
	withSudoPath(t, "sudo", nil)

	_, args, err := WrapForBecome(Become{Enabled: true, User: "deploy"}, "apt-get", []string{"install", "-y", "pkg"})
	if err != nil {
		t.Fatal(err)
	}
	assertArgs(t, args, []string{"-u", "deploy", "apt-get", "install", "-y", "pkg"})
}

func TestWrapForBecomeRootUserOmitsDashU(t *testing.T) {
	withSudoPath(t, "sudo", nil)

	_, args, err := WrapForBecome(Become{Enabled: true, User: "root"}, "apt-get", []string{"install"})
	if err != nil {
		t.Fatal(err)
	}
	assertArgs(t, args, []string{"apt-get", "install"})
}

func TestWrapForBecomeFailsWhenSudoMissing(t *testing.T) {
	withSudoPath(t, "", errors.New("not found"))

	if _, _, err := WrapForBecome(Become{Enabled: true}, "apt-get", nil); err == nil {
		t.Fatal("expected error when sudo is missing")
	}
}
