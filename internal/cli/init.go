package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/facts"
	"github.com/TacoContent/ironstate/internal/scan"
)

// minimalDocument is every generated YAML file's starter content -
// deliberately empty, just enough to be a valid site/overlay document.
const minimalDocument = "---\nvars: {}\ntasks: []\n"

// initScaffoldFile is one file 'init' creates, relative to the playbook
// root - content is empty for a placeholder like '.gitkeep'.
type initScaffoldFile struct {
	path    string
	content string
}

var gatherScanItems = func(progress func(name string, index, total int)) ([]scan.Item, error) {
	return scan.NewRegistry().ScanAllWithProgress(progress)
}

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init [playbook-name]",
		Short: "Scaffold a new playbook's starter file/directory structure",
		Long: `Creates a new playbook (or scaffolds the current directory, if no name is
given) with the base file/directory structure ironstate expects - see the
README's "Playbooks" section:

  <playbook>/
  |-- main.yml
  |-- filters/
  |-- roles/
  |-- tasks/
  |-- packages/
  |-- hosts/<machine-name>.yml
  '-- variables/<user-name>.yml

Every generated YAML file is intentionally minimal ('vars: {}' / 'tasks: []');
<machine-name>/<user-name> come from this machine's own gathered facts (see
'ironstate doctor'), lowercased. An already-existing file or directory is left
untouched rather than overwritten, so running 'init' again is safe.

Use '--scan' to populate the generated playbook from the current system state
instead of creating only placeholders.`,
		Args: cobra.MaximumNArgs(1),
		RunE: runInit,
	}
	cmd.Flags().Bool("scan", false, "populate the generated playbook from the current system state")
	return cmd
}

func runInit(cmd *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 && args[0] != "" {
		root = args[0]
	}

	scanEnabled, err := cmd.Flags().GetBool("scan")
	if err != nil {
		return NewLoadError(err)
	}
	if scanEnabled {
		progress := newProgressReporter()
		progress.Start()
		defer progress.Stop()

		items, err := gatherScanItems(func(name string, index, total int) {
			if total <= 0 {
				return
			}
			progress.Step("scanning", index, total, name)
		})
		if err != nil {
			return NewRunError(err)
		}
		progress.Message("populated playbook from current system state")
		if err := scan.GeneratePlaybook(root, items); err != nil {
			return NewLoadError(fmt.Errorf("populate playbook from scan: %w", err))
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "populated %s from current system state\n", root); err != nil {
			return NewRunError(err)
		}
		if _, err := fmt.Fprintf(cmd.OutOrStdout(), "Use ironstate --playbook %s to review the generated playbook.\n", filepath.Join(root, "main.yml")); err != nil {
			return NewRunError(err)
		}
		return nil
	}

	hostFacts := facts.Gather()
	machineName := factString(hostFacts, "computer_name", "host")
	osFamily := factString(hostFacts, "os_family", "linux")
	userName := factString(hostFacts, "user_name", "user")

	dirs := []string{
		root,
		filepath.Join(root, "roles"),
		filepath.Join(root, "tasks"),
		filepath.Join(root, "filters"),
		filepath.Join(root, "packages"),
		filepath.Join(root, "hosts"),
		filepath.Join(root, "variables"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil { //nolint:gosec // matches ironstate.ps1's own directories (not secrets), no tighter mode intended
			return NewLoadError(fmt.Errorf("create %s: %w", dir, err))
		}
	}

	files := []initScaffoldFile{
		{filepath.Join(root, "main.yml"), minimalDocument},
		{filepath.Join(root, "hosts", machineName+".yml"), minimalDocument},
		{filepath.Join(root, "hosts", osFamily+".yml"), minimalDocument},
		{filepath.Join(root, "variables", userName+".yml"), minimalDocument},
		{filepath.Join(root, "roles", ".gitkeep"), ""},
		{filepath.Join(root, "filters", ".gitkeep"), ""},
		{filepath.Join(root, "tasks", ".gitkeep"), ""},
		{filepath.Join(root, "packages", ".gitkeep"), ""},
	}

	for _, f := range files {
		if _, err := os.Stat(f.path); err == nil {
			cmd.Printf("skip    %s (already exists)\n", f.path)
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil { //nolint:gosec // matches ironstate.ps1's own file permissions, no tighter mode intended
			return NewLoadError(fmt.Errorf("write %s: %w", f.path, err))
		}
		cmd.Printf("created %s\n", f.path)
	}

	cmd.Printf("\nPlaybook ready. Try:\n  ironstate --playbook %s\n", filepath.Join(root, "main.yml"))
	return nil
}

// factString reads a string-valued fact, lowercased, falling back to
// fallback when the fact is missing/empty/non-string.
func factString(f map[string]any, key, fallback string) string {
	if v, ok := f[key].(string); ok && v != "" {
		return strings.ToLower(v)
	}
	return fallback
}
