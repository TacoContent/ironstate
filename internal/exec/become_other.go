//go:build !windows

package exec

func isWindowsAdmin() bool { return false }
