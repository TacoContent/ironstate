//go:build darwin

package facts

import "os"

// isAdmin treats an effective UID of 0 (root) as macOS's equivalent of
// Windows' "elevated" check - there's no UAC-style distinction on macOS.
func isAdmin() bool { return os.Geteuid() == 0 }

// osVersion reports the macOS product version verbatim (e.g. "14.5",
// "15.1.1") via 'sw_vers -productVersion' - the standard, documented way
// to get this without cgo/private syscalls.
func osVersion() string {
	out, err := runVersionProbe("sw_vers", "-productVersion")
	if err != nil {
		return ""
	}
	return firstLine(out)
}

// osBuildNumber has no equivalent single number on macOS.
func osBuildNumber() uint32 { return 0 }
