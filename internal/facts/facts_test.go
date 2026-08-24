package facts

import (
	"runtime"
	"testing"
)

func TestGatherShape(t *testing.T) {
	origRunner := pwshRunner
	pwshRunner = func() (string, error) { return "PowerShell 7.4.6\n", nil }
	defer func() { pwshRunner = origRunner }()

	f := Gather()
	for _, key := range []string{
		"computer_name", "user_name", "home", "os_version", "os_build", "is_admin",
		"shell_pwsh", "pwsh_version",
		"shell_bash", "bash_version",
		"shell_zsh", "zsh_version",
		"shell_fish", "fish_version",
		"shell_nu", "nu_version",
		"platform", "arch", "os_family",
	} {
		if _, ok := f[key]; !ok {
			t.Errorf("missing fact %q", key)
		}
	}
	if _, ok := f["is_admin"].(bool); !ok {
		t.Errorf("is_admin type = %T, want bool", f["is_admin"])
	}
	if _, ok := f["os_build"].(float64); !ok {
		t.Errorf("os_build type = %T, want float64", f["os_build"])
	}
	if f["shell_pwsh"] != true {
		t.Errorf("shell_pwsh = %v, want true", f["shell_pwsh"])
	}
	if f["pwsh_version"] != "PowerShell 7.4.6" {
		t.Errorf("pwsh_version = %q", f["pwsh_version"])
	}
	if f["platform"] != runtime.GOOS {
		t.Errorf("platform = %v, want %q", f["platform"], runtime.GOOS)
	}
	if f["arch"] != runtime.GOARCH {
		t.Errorf("arch = %v, want %q", f["arch"], runtime.GOARCH)
	}
}

func TestOSFamily(t *testing.T) {
	cases := map[string]string{
		"windows": "windows",
		"linux":   "unix",
		"darwin":  "unix",
		"plan9":   "plan9",
	}
	for goos, want := range cases {
		if got := osFamily(goos); got != want {
			t.Errorf("osFamily(%q) = %q, want %q", goos, got, want)
		}
	}
}

func TestPwshVersionEmptyWhenNotFound(t *testing.T) {
	origRunner := pwshRunner
	pwshRunner = func() (string, error) { return "", errNotFound }
	defer func() { pwshRunner = origRunner }()

	if got := pwshVersion(); got != "" {
		t.Errorf("pwshVersion() = %q, want empty", got)
	}
}

func TestShellVersionsEmptyWhenNotFound(t *testing.T) {
	origBash, origZsh, origFish, origNu := bashRunner, zshRunner, fishRunner, nuRunner
	bashRunner = func() (string, error) { return "", errNotFound }
	zshRunner = func() (string, error) { return "", errNotFound }
	fishRunner = func() (string, error) { return "", errNotFound }
	nuRunner = func() (string, error) { return "", errNotFound }
	defer func() { bashRunner, zshRunner, fishRunner, nuRunner = origBash, origZsh, origFish, origNu }()

	for name, got := range map[string]string{"bash": bashVersion(), "zsh": zshVersion(), "fish": fishVersion(), "nu": nuVersion()} {
		if got != "" {
			t.Errorf("%sVersion() = %q, want empty", name, got)
		}
	}
}

func TestFirstLineKeepsOnlyTheSummaryLine(t *testing.T) {
	multiline := "GNU bash, version 5.3.9(1)-release (x86_64-pc-linux-gnu)\nCopyright (C) 2024 Free Software Foundation, Inc.\n"
	if got := firstLine(multiline); got != "GNU bash, version 5.3.9(1)-release (x86_64-pc-linux-gnu)" {
		t.Errorf("firstLine(...) = %q", got)
	}
}

func TestGatherReportsShellVersionsAsNullWhenAbsent(t *testing.T) {
	origBash, origZsh, origFish, origNu := bashRunner, zshRunner, fishRunner, nuRunner
	bashRunner = func() (string, error) { return "", errNotFound }
	zshRunner = func() (string, error) { return "", errNotFound }
	fishRunner = func() (string, error) { return "", errNotFound }
	nuRunner = func() (string, error) { return "", errNotFound }
	defer func() { bashRunner, zshRunner, fishRunner, nuRunner = origBash, origZsh, origFish, origNu }()

	f := Gather()
	for _, key := range []string{"bash_version", "zsh_version", "fish_version", "nu_version"} {
		if f[key] != nil {
			t.Errorf("%s = %v, want nil", key, f[key])
		}
	}
	for _, key := range []string{"shell_bash", "shell_zsh", "shell_fish", "shell_nu"} {
		if f[key] != false {
			t.Errorf("%s = %v, want false", key, f[key])
		}
	}
}

type notFoundErr struct{}

func (notFoundErr) Error() string { return "not found" }

var errNotFound = notFoundErr{}
