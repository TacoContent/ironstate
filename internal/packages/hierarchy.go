package packages

import (
	"fmt"
	"os"
	"path/filepath"
)

// LoadHierarchy loads baseFile then merges the host- (hosts/<COMPUTERNAME>.yml)
// and user- (variables/<USERNAME>.yml) specific overlays, in that order,
// if present — ports Packages.psm1's Import-PackagesHierarchy. All three
// files resolve their own 'copy.src'/'shell.script' paths relative to the
// same root (baseFile's directory), regardless of which subdirectory the
// overlay file itself lives in.
func LoadHierarchy(baseFile string) (any, error) {
	dir := filepath.Dir(baseFile)
	data, err := LoadFile(baseFile, dir)
	if err != nil {
		return nil, err
	}

	var overlayFiles []string
	if computerName := os.Getenv("COMPUTERNAME"); computerName != "" {
		overlayFiles = append(overlayFiles, filepath.Join(dir, "hosts", computerName+".yml"))
	}
	if userName := os.Getenv("USERNAME"); userName != "" {
		overlayFiles = append(overlayFiles, filepath.Join(dir, "variables", userName+".yml"))
	}

	for _, file := range overlayFiles {
		if _, err := os.Stat(file); err != nil { //nolint:gosec // fixed overlay locations derived from COMPUTERNAME/USERNAME, same trust boundary as the rest of this tool
			continue
		}
		overlay, err := LoadFile(file, dir)
		if err != nil {
			return nil, err
		}
		baseMap, ok := data.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("base document must use the explicit 'tasks:'/'vars:' mapping form to merge overlay %s", file)
		}
		overlayMap, ok := overlay.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("overlay file %s must use the explicit 'tasks:'/'vars:' mapping form", file)
		}
		data = MergeDocuments(baseMap, overlayMap)
	}
	return data, nil
}
