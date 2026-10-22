//go:build !windows

package exec

// RefreshPath is a no-op outside Windows, where installers do not publish PATH
// changes through the registry.
func RefreshPath() (bool, error) { return false, nil }
