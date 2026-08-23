package facts

import "testing"

func TestGatherShape(t *testing.T) {
	origRunner := pwshRunner
	pwshRunner = func() (string, error) { return "PowerShell 7.4.6\n", nil }
	defer func() { pwshRunner = origRunner }()

	f := Gather()
	for _, key := range []string{"computer_name", "user_name", "home", "os_version", "os_build", "is_admin", "pwsh_version"} {
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
	if f["pwsh_version"] != "PowerShell 7.4.6" {
		t.Errorf("pwsh_version = %q", f["pwsh_version"])
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

type notFoundErr struct{}

func (notFoundErr) Error() string { return "not found" }

var errNotFound = notFoundErr{}
