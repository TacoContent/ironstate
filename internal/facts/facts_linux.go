//go:build linux

package facts

import "os"

// isAdmin treats an effective UID of 0 (root) as Linux's equivalent of
// Windows' "elevated" check - there's no UAC-style distinction on Linux.
func isAdmin() bool { return os.Geteuid() == 0 }

// osVersion reports /etc/os-release's VERSION_ID verbatim (e.g. Ubuntu's
// "22.04", Debian's "12", Alpine's "3.19.1") since that's the version
// users actually mean by "the OS version" on Linux - falling back to the
// kernel release (e.g. "6.8.0-31-generic") via 'uname -r' for rolling
// releases with no VERSION_ID at all (e.g. Arch Linux). Reported as-is,
// not reformatted/reparsed into a fixed major.minor.build shape.
func osVersion() string {
	if v := distroVersionID(); v != "" {
		return v
	}
	if out, err := runVersionProbe("uname", "-r"); err == nil {
		return firstLine(out)
	}
	return ""
}

// osBuildNumber has no equivalent single number on Linux.
func osBuildNumber() uint32 { return 0 }
