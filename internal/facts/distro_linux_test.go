//go:build linux

package facts

import (
	"os"
	"path/filepath"
	"testing"
)

func writeOSRelease(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func withOSReleasePaths(t *testing.T, paths ...string) {
	t.Helper()
	orig := osReleasePaths
	osReleasePaths = paths
	t.Cleanup(func() { osReleasePaths = orig })
}

func TestDistroReadsIDField(t *testing.T) {
	cases := map[string]struct{ content, want string }{
		"ubuntu":  {"NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\n", "ubuntu"},
		"debian":  {"NAME=\"Debian GNU/Linux\"\nID=debian\n", "debian"},
		"arch":    {"NAME=\"Arch Linux\"\nID=arch\n", "archlinux"},
		"manjaro": {"NAME=\"Manjaro Linux\"\nID=manjaro\n", "manjaro"},
		"alpine":  {"NAME=\"Alpine Linux\"\nID=alpine\n", "alpine"},
		"rhel":    {"NAME=\"Red Hat Enterprise Linux\"\nID=rhel\n", "redhat"},
		"mint":    {"NAME=\"Linux Mint\"\nID=linuxmint\nID_LIKE=ubuntu\n", "linuxmint"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			withOSReleasePaths(t, writeOSRelease(t, tc.content))
			if got := distro(); got != tc.want {
				t.Errorf("distro() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDistroPrefersIDAndFamilyPrefersIDLike(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "ID=manjaro\nID_LIKE=\"arch\"\n"))
	if got := distro(); got != "manjaro" {
		t.Errorf("distro() = %q, want manjaro", got)
	}
	if got := distroFamily(); got != "archlinux" {
		t.Errorf("distroFamily() = %q, want archlinux", got)
	}
}

func TestDistroAndFamilyFallBackToIDLike(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "ID_LIKE=\"arch linux\"\n"))
	if got := distro(); got != "archlinux" {
		t.Errorf("distro() = %q, want archlinux", got)
	}
	if got := distroFamily(); got != "archlinux" {
		t.Errorf("distroFamily() = %q, want archlinux", got)
	}
}

func TestDistroFallsBackToIDLikeWhenIDMissing(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "NAME=\"Some Derivative\"\nID_LIKE=\"ubuntu debian\"\n"))
	if got := distro(); got != "ubuntu" {
		t.Errorf("distro() = %q, want %q", got, "ubuntu")
	}
}

func TestDistroFallsBackToSecondPathWhenFirstMissing(t *testing.T) {
	withOSReleasePaths(t, filepath.Join(t.TempDir(), "missing"), writeOSRelease(t, "ID=fedora\n"))
	if got := distro(); got != "fedora" {
		t.Errorf("distro() = %q, want %q", got, "fedora")
	}
}

func TestDistroEmptyWhenNoFileFound(t *testing.T) {
	withOSReleasePaths(t, filepath.Join(t.TempDir(), "missing"))
	if got := distro(); got != "" {
		t.Errorf("distro() = %q, want empty", got)
	}
}

func TestOSFamilyPrefersIDLikeOnLinux(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "NAME=\"Manjaro\"\nID=manjaro\nID_LIKE=arch\n"))
	if got := osFamily("linux"); got != "archlinux" {
		t.Errorf("osFamily(%q) = %q, want %q", "linux", got, "archlinux")
	}
	if got := distribution("linux"); got != "manjaro" {
		t.Errorf("distribution(%q) = %q, want %q", "linux", got, "manjaro")
	}
}

func TestOSFamilyFallsBackToLinuxWhenDistroUnknown(t *testing.T) {
	withOSReleasePaths(t, filepath.Join(t.TempDir(), "missing"))
	if got := osFamily("linux"); got != "linux" {
		t.Errorf("osFamily(%q) = %q, want %q", "linux", got, "linux")
	}
	if got := distribution("linux"); got != "linux" {
		t.Errorf("distribution(%q) = %q, want %q", "linux", got, "linux")
	}
}

func TestDistroVersionIDReadsField(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "NAME=\"Ubuntu\"\nID=ubuntu\nVERSION_ID=\"22.04\"\n"))
	if got := distroVersionID(); got != "22.04" {
		t.Errorf("distroVersionID() = %q, want %q", got, "22.04")
	}
}

func TestDistroVersionIDEmptyWhenMissing(t *testing.T) {
	withOSReleasePaths(t, writeOSRelease(t, "NAME=\"Arch Linux\"\nID=arch\n"))
	if got := distroVersionID(); got != "" {
		t.Errorf("distroVersionID() = %q, want empty", got)
	}
}
