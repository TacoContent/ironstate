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
// Windows, since GetVersionEx is subject to app-compat shimming), as
// "major.minor.build" - Windows has no single native version string, so
// this is assembled rather than reported verbatim like the other
// platforms.
func osVersion() string {
	info := windows.RtlGetVersion()
	return itoa(info.MajorVersion) + "." + itoa(info.MinorVersion) + "." + itoa(info.BuildNumber)
}

// osBuildNumber mirrors os_build - only meaningful on Windows, where
// RtlGetVersion reports a real build number.
func osBuildNumber() uint32 {
	return windows.RtlGetVersion().BuildNumber
}

func itoa(v uint32) string {
	if v == 0 {
		return "0"
	}
	var digits [10]byte
	i := len(digits)
	for v > 0 {
		i--
		digits[i] = byte('0' + v%10)
		v /= 10
	}
	return string(digits[i:])
}
