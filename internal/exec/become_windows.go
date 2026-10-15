//go:build windows

package exec

import "golang.org/x/sys/windows"

func isWindowsAdmin() bool { return windows.GetCurrentProcessToken().IsElevated() }
