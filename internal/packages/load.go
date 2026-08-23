package packages

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TacoContent/ironstate/internal/model"
)

// LoadFile loads a single YAML file and resolves its 'copy.src'/
// 'shell.script'/etc. paths relative to baseDir (the file's own directory
// when baseDir is empty) — ports Packages.psm1's Import-PackagesFile.
func LoadFile(path string, baseDir string) (any, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is a configured YAML document location, same trust boundary as the rest of this tool
	if err != nil {
		return nil, fmt.Errorf("packages file not found: %s: %w", path, err)
	}
	if baseDir == "" {
		baseDir = filepath.Dir(path)
	}

	doc, err := model.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	ResolveRelativePathsInPlace(doc, baseDir)
	return doc, nil
}
