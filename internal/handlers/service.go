package handlers

import (
	"errors"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// serviceHandler is a scan-only module: internal/scan's Registry
// discovers system services through it (see Scan/ScanRole below), but
// there is no install-side "service" module yet - starting, stopping, or
// enabling a service isn't implemented by any handler today, so Test/
// Describe/Install/Uninstall all report the module as unsupported rather
// than silently doing nothing. It exists purely to give service
// discovery a home in the handlers package alongside every other
// ScanCapable module, instead of internal/scan hardcoding its own
// separate scanner.
type serviceHandler struct{}

var errServiceUnsupported = errors.New("the 'service' module has no install/uninstall support yet - it is scan-only")

func (serviceHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	return false, errServiceUnsupported
}

func (serviceHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	return "", errServiceUnsupported
}

func (serviceHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errServiceUnsupported
}

func (serviceHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	return engine.ExecResult{}, errServiceUnsupported
}

// ScanRole implements engine.ScanCapable - discovered services seed
// roles/system/services in a generated playbook (see internal/scan).
func (serviceHandler) ScanRole() string { return "roles/system/services" }

// serviceEntry is one systemd unit discovered via 'systemctl
// list-unit-files' - see discoverServices.
type serviceEntry struct {
	Name string
}

// Scan implements engine.ScanCapable: discovers the host's systemd
// services - ports the scanning logic that used to live in
// internal/scan's serviceScanner. Windows has no equivalent
// implementation here (Win32_Service discovery was unreachable dead code
// in the original scan.go - internal/scan never registered a Windows
// service scanner in the first place), so it reports nothing there.
func (serviceHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS == "windows" {
		return nil, nil
	}
	entries, err := discoverServices()
	if err != nil {
		return nil, err
	}
	out := make([]engine.ScanItem, 0, len(entries))
	for _, e := range entries {
		if e.Name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "service",
			Name:   e.Name,
			Config: map[string]any{"name": e.Name, "state": "started", "enabled": true},
			Tags:   []string{"system", "services"},
		})
	}
	return out, nil
}

func discoverServices() ([]serviceEntry, error) {
	result, err := runner.Run("systemctl", []string{"list-unit-files", "--type=service", "--no-pager", "--plain"})
	if err != nil {
		return nil, err
	}
	entries := make([]serviceEntry, 0)
	for _, line := range strings.Split(result.Stdout, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] == "UNIT" || strings.Contains(line, "UNIT FILE") {
			continue
		}
		entries = append(entries, serviceEntry{Name: fields[0]})
	}
	return entries, nil
}
