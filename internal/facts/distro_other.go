//go:build !linux

package facts

// distro is only meaningful on Linux, where /etc/os-release identifies
// the specific distribution; every other platform reports "".
func distro() string { return "" }

// distroFamily is only meaningful on Linux, where /etc/os-release
// identifies the operating system family.
func distroFamily() string { return "" }
