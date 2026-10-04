//go:build windows

package remoteexec

// controlPath is empty: the Windows OpenSSH client can't multiplex.
func controlPath() string { return "" }
