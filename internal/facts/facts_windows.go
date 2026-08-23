//go:build windows

package facts

import "golang.org/x/sys/windows"

// isAdmin reports whether the current process token is elevated,
// mirroring Facts.psm1's WindowsPrincipal.IsInRole(Administrator) check.
func isAdmin() bool {
	token := windows.GetCurrentProcessToken()
	return token.IsElevated()
}

// osVersion mirrors [System.Environment]::OSVersion.Version via
// RtlGetVersion (the documented way to get the real OS version on modern
// Windows, since GetVersionEx is subject to app-compat shimming).
func osVersion() (major, minor, build uint32) {
	info := windows.RtlGetVersion()
	return info.MajorVersion, info.MinorVersion, info.BuildNumber
}
