package cli

import (
	"os/exec"
	"sort"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/filters"
)

// checkedCommands are looked up on PATH by `ironstate doctor`. This list
// mirrors ironstate.ps1's package-manager modules plus the one remaining
// optional PowerShell dependency (shell.host: pwsh — see
// docs/plans/go-rewrite.md §11).
var checkedCommands = []string{
	"winget", "choco", "pipx", "npm", "cargo", "go", "gem", "eget", "pwsh",
}

func newDoctorCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check PATH for package managers and optional runtime dependencies",
		RunE: func(cmd *cobra.Command, _ []string) error {
			for _, name := range checkedCommands {
				if path, err := exec.LookPath(name); err == nil {
					cmd.Printf("[ok]      %-8s %s\n", name, path)
				} else {
					cmd.Printf("[missing] %-8s not found on PATH\n", name)
				}
			}

			dir, _ := cmd.Flags().GetString("filters-dir")
			interpreters := filters.DefaultInterpreters()
			r := filters.New()
			pool, discovered, err := filters.DiscoverScriptFilters(r, dir, interpreters)
			if err != nil {
				return err
			}
			defer func() { _ = pool.Close() }()

			sort.Strings(discovered)
			cmd.Printf("\nscript filters discovered under %s:\n", dir)
			if len(discovered) == 0 {
				cmd.Println("  (none)")
			}
			for _, name := range discovered {
				cmd.Printf("  %s\n", name)
			}
			return nil
		},
	}
	cmd.Flags().String("filters-dir", "filters", "directory to scan for external script filters")
	return cmd
}
