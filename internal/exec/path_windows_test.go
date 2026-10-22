//go:build windows

package exec

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshPathPreservesSessionAndAppendsNewEntries(t *testing.T) {
	t.Setenv("PATH", `C:\session;C:\Windows;C:\existing\`)
	paths := []string{`c:\WINDOWS\;C:\machine;;`, `"c:\EXISTING";C:\scoop\shims`}
	read := func() ([]string, error) { return paths, nil }
	changed, err := refreshPath(read)
	want := `C:\session;C:\Windows;C:\existing\;C:\machine;C:\scoop\shims`
	if err != nil || !changed || os.Getenv("PATH") != want {
		t.Fatalf("changed=%v err=%v PATH=%q, want %q", changed, err, os.Getenv("PATH"), want)
	}
	changed, err = refreshPath(read)
	if err != nil || changed || os.Getenv("PATH") != want {
		t.Fatalf("second refresh: changed=%v err=%v PATH=%q", changed, err, os.Getenv("PATH"))
	}
	paths = append(paths, `C:\new-tool`)
	changed, err = refreshPath(read)
	if err != nil || !changed || os.Getenv("PATH") != want+`;C:\new-tool` {
		t.Fatalf("installer refresh: changed=%v err=%v PATH=%q", changed, err, os.Getenv("PATH"))
	}
}

func TestRefreshPathEmptyAndFailedReads(t *testing.T) {
	for _, current := range []string{"", `C:\session;`} {
		t.Run(current, func(t *testing.T) {
			t.Setenv("PATH", current)
			changed, err := refreshPath(func() ([]string, error) { return nil, nil })
			if err != nil || changed || os.Getenv("PATH") != current {
				t.Fatalf("empty read: changed=%v err=%v PATH=%q", changed, err, os.Getenv("PATH"))
			}
			readErr := errors.New("registry access denied")
			changed, err = refreshPath(func() ([]string, error) { return nil, readErr })
			if !errors.Is(err, readErr) || changed || os.Getenv("PATH") != current {
				t.Fatalf("failed read: changed=%v err=%v PATH=%q", changed, err, os.Getenv("PATH"))
			}
			changed, err = refreshPath(func() ([]string, error) { return []string{`C:\new`}, nil })
			want := current + `C:\new`
			if current != "" && !strings.HasSuffix(current, ";") {
				want = current + `;C:\new`
			}
			if err != nil || !changed || os.Getenv("PATH") != want {
				t.Fatalf("append: changed=%v err=%v PATH=%q, want %q", changed, err, os.Getenv("PATH"), want)
			}
		})
	}
}

func TestRefreshedPathReachesPowerShellChildren(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh is not installed")
	}
	t.Setenv("PATH", t.TempDir())
	shims := t.TempDir()
	run := func(command string) Result {
		t.Helper()
		result, err := (realRunner{}).Run(pwsh, []string{"-NoLogo", "-NoProfile", "-Command", command})
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	if result := run("Get-Command scoop -ErrorAction SilentlyContinue"); result.RC == 0 {
		t.Fatalf("scoop unexpectedly available before installation: %#v", result)
	}
	if err := os.WriteFile(filepath.Join(shims, "scoop.ps1"), []byte("Write-Output ('scoop ' + ($args -join ' '))"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := refreshPath(func() ([]string, error) { return []string{shims}, nil })
	if err != nil || !changed {
		t.Fatalf("refresh: changed=%v err=%v", changed, err)
	}
	for _, command := range []string{
		"Get-Command scoop -ErrorAction SilentlyContinue",
		"scoop bucket list",
		"scoop bucket add camalot https://example.invalid/bucket",
	} {
		result := run(command)
		if result.RC != 0 || !strings.Contains(result.Stdout, "scoop") {
			t.Fatalf("%s: %#v", command, result)
		}
	}
}
