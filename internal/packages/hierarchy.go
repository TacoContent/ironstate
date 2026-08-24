package packages

import (
	"os"
	"path/filepath"
)

// LoadHierarchy loads baseFile, then layers (in order): hosts/'s default
// (main.yml) plus chained hostname/architecture/os_family/platform
// overlays, variables/'s default (main.yml) plus the same chained
// overlays, then the legacy username-keyed variables/<USERNAME>.yml
// overlay — ports Packages.psm1's Import-PackagesHierarchy, extended per
// docs/plans/notes.md so a host/variant-specific overlay isn't limited to
// a single hosts/<COMPUTERNAME>.yml file. Every overlay file resolves its
// own 'copy.src'/'shell.script' paths relative to the same root
// (baseFile's directory), regardless of which subdirectory the overlay
// file itself lives in.
func LoadHierarchy(baseFile string, hostFacts map[string]any) (any, error) {
	dir := filepath.Dir(baseFile)
	data, err := LoadFile(baseFile, dir)
	if err != nil {
		return nil, err
	}

	hostsDir := filepath.Join(dir, "hosts")
	if data, err = MergeDefaultOverlay(data, hostsDir, dir); err != nil {
		return nil, err
	}
	if data, err = MergeChainOverlays(data, hostsDir, dir, hostFacts); err != nil {
		return nil, err
	}

	varsDir := filepath.Join(dir, "variables")
	if data, err = MergeDefaultOverlay(data, varsDir, dir); err != nil {
		return nil, err
	}
	if data, err = MergeChainOverlays(data, varsDir, dir, hostFacts); err != nil {
		return nil, err
	}

	if userName := os.Getenv("USERNAME"); userName != "" {
		if userFile := findChainFile(varsDir, userName); userFile != "" {
			if data, err = mergeOverlayFile(data, userFile, dir); err != nil {
				return nil, err
			}
		}
	}
	return data, nil
}
