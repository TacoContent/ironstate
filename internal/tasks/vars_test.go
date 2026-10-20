package tasks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
)

func TestPackageVariableNamespaceInLoops(t *testing.T) {
	for _, module := range []string{"include", "uses"} {
		t.Run(module, func(t *testing.T) {
			root := t.TempDir()
			pkgDir := filepath.Join(root, "example")
			if err := os.MkdirAll(pkgDir, 0o700); err != nil {
				t.Fatal(err)
			}
			content := `vars:
  defaults:
    message: fallback
tasks:
  - items: "${{ vars.defaults | default({}) }}"
    log:
      message: "${{ vars.defaults.message }}"
`
			if err := os.WriteFile(filepath.Join(pkgDir, "main.yml"), []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			spec := map[string]any{"name": "example"}
			if module == "uses" {
				spec = map[string]any{"remote": "example"}
			}
			leaves, err := Expand([]any{map[string]any{module: spec}}, Options{
				ModuleNames: []string{"log"}, PackagesRoot: root, Filters: filters.New(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(leaves) != 1 || leaves[0].Item["message"] != "fallback" {
				t.Fatalf("leaves = %#v, want one leaf using package defaults", leaves)
			}
			if model.AsMap(leaves[0].ItemCtx["item"])["message"] != "fallback" {
				t.Fatalf("loop item lost its default map: %#v", leaves[0].ItemCtx)
			}
		})
	}
}
