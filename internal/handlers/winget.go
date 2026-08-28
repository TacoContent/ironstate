package handlers

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/TacoContent/ironstate/internal/engine"
)

// wingetHandler ports Handlers/Winget.psm1 (Windows Package Manager).
type wingetHandler struct{}

func (wingetHandler) Test(item map[string]any, name string, ctx engine.Context) (bool, error) {
	pkg := getString(item, "package")
	result, err := runner.Run("winget", []string{"list", "--id", pkg, "--exact", "--accept-source-agreements"})
	if err != nil {
		return false, nil //nolint:nilerr // winget not being on PATH is filtered out before Test ever runs; any other invocation failure just means "not installed"
	}
	out := result.Stdout + result.Stderr
	return result.RC == 0 && !strings.Contains(out, "No installed package found"), nil
}

func (wingetHandler) Describe(item map[string]any, action engine.Action, ctx engine.Context) (string, error) {
	pkg := getString(item, "package")
	if action == engine.ActionUninstall {
		return "winget uninstall --id " + pkg + " --exact", nil
	}
	desc := "winget install --id " + pkg + " --exact"
	if source := getString(item, "source"); source != "" {
		desc += " --source " + source
	}
	if override := getString(item, "override"); override != "" {
		desc += " --override " + override
	}
	return desc, nil
}

func (wingetHandler) Install(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	args := []string{"install", "--id", pkg, "--exact", "--accept-source-agreements", "--accept-package-agreements"}
	if source := getString(item, "source"); source != "" {
		args = append(args, "--source", source)
	}
	if override := getString(item, "override"); override != "" {
		args = append(args, "--override", override)
	}
	result := runExternalCommand("winget", args)
	if result.RC != 0 {
		engine.Warn("winget install %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

func (wingetHandler) Uninstall(item map[string]any, name string, ctx engine.Context) (engine.ExecResult, error) {
	pkg := getString(item, "package")
	result := runExternalCommand("winget", []string{"uninstall", "--id", pkg, "--exact"})
	if result.RC != 0 {
		engine.Warn("winget uninstall %s exited with code %d", pkg, result.RC)
	}
	return result, nil
}

// ScanRole implements engine.ScanCapable - discovered packages seed
// roles/packages in a generated playbook (see internal/scan).
func (wingetHandler) ScanRole() string { return "roles/packages" }

// Scan implements engine.ScanCapable: discovers packages winget (or the
// Microsoft Store source it also manages) has installed, via 'winget
// export' - ports the scanning logic that used to live in internal/
// scan's packageScanner.
func (wingetHandler) Scan(ctx engine.Context) ([]engine.ScanItem, error) {
	if runtime.GOOS != "windows" {
		return nil, nil
	}
	packages, err := discoverWingetPackagesFromExport()
	if err != nil {
		return nil, err
	}
	out := make([]engine.ScanItem, 0, len(packages))
	for _, p := range wingetPackagesToEntries(packages) {
		if p.Name == "" {
			continue
		}
		out = append(out, engine.ScanItem{
			Module: "winget",
			Name:   p.Name,
			Config: map[string]any{"package": p.Identifier, "state": "present", "source": p.Source},
			Tags:   []string{"packages"},
		})
	}
	return out, nil
}

// wingetExportPackage is one package entry parsed from 'winget export'.
type wingetExportPackage struct {
	Identifier string
	Version    string
	Source     string
}

type wingetExportFile struct {
	Sources []struct {
		Packages []struct {
			PackageIdentifier string `json:"PackageIdentifier"`
			Version           string `json:"Version"`
		} `json:"Packages"`
	} `json:"Sources"`
}

func discoverWingetPackagesFromExport() ([]wingetExportPackage, error) {
	entries := make([]wingetExportPackage, 0)
	for _, source := range []string{"winget", "msstore"} {
		packages, err := exportWingetPackages(source)
		if err != nil {
			continue
		}
		entries = append(entries, packages...)
	}
	return entries, nil
}

func exportWingetPackages(source string) ([]wingetExportPackage, error) {
	tempFile, err := os.CreateTemp("", "ironstate-winget-export-*.json")
	if err != nil {
		return nil, err
	}
	path := tempFile.Name()
	_ = tempFile.Close()
	defer func() { _ = os.Remove(path) }()

	result, err := runner.Run("winget", []string{"export", "--source", source, "--output", path, "--include-versions", "--accept-source-agreements"})
	if err != nil {
		return nil, fmt.Errorf("winget export %s: %w: %s", source, err, strings.TrimSpace(result.Stdout+result.Stderr))
	}
	data, err := os.ReadFile(path) // #nosec G304 -- winget export writes to a temp file we created ourselves
	if err != nil {
		return nil, err
	}
	var exported wingetExportFile
	if err := json.Unmarshal(data, &exported); err != nil {
		return nil, err
	}
	items := make([]wingetExportPackage, 0)
	for _, sourceBlock := range exported.Sources {
		for _, pkg := range sourceBlock.Packages {
			if pkg.PackageIdentifier == "" {
				continue
			}
			items = append(items, wingetExportPackage{Identifier: pkg.PackageIdentifier, Version: pkg.Version, Source: source})
		}
	}
	return items, nil
}

// wingetPackage is a winget-export entry resolved to a display name -
// see wingetPackagesToEntries.
type wingetPackage struct {
	Name       string
	Identifier string
	Source     string
}

// wingetPackagesToEntries resolves each export entry's display name,
// looking up the Microsoft Store's friendly name for 'msstore'-sourced
// packages (winget's own export only has their opaque product IDs).
func wingetPackagesToEntries(packages []wingetExportPackage) []wingetPackage {
	entries := make([]wingetPackage, 0, len(packages))
	msstoreNames := map[string]string{}
	for _, pkg := range packages {
		name := pkg.Identifier
		if strings.EqualFold(pkg.Source, "msstore") {
			if cached, ok := msstoreNames[pkg.Identifier]; ok {
				name = cached
			} else if displayName := wingetDisplayNameLookup(pkg.Identifier); displayName != "" {
				name = displayName
				msstoreNames[pkg.Identifier] = displayName
			}
		}
		entries = append(entries, wingetPackage{Name: name, Identifier: pkg.Identifier, Source: pkg.Source})
	}
	return entries
}

var wingetDisplayNameLookup = wingetDisplayName

func wingetDisplayName(identifier string) string {
	result, err := runner.Run("winget", []string{"list", identifier, "--accept-source-agreements"})
	if err != nil {
		return ""
	}
	return parseWingetDisplayName(result.Stdout, identifier)
}

func parseWingetDisplayName(out, identifier string) string {
	for _, line := range strings.Split(out, "\n") {
		trimmed := strings.TrimSpace(strings.TrimRight(line, "\r"))
		if trimmed == "" || strings.HasPrefix(trimmed, "Name") || strings.Trim(trimmed, "-") == "" {
			continue
		}
		idx := strings.Index(line, identifier)
		if idx <= 0 {
			continue
		}
		name := strings.TrimSpace(line[:idx])
		if name != "" {
			return name
		}
	}
	return ""
}
