package cli

import (
	"os/exec"

	"github.com/spf13/cobra"
)

// checkedCommands are looked up on PATH by `ironstate doctor`. This list
// mirrors ironstate.ps1's package-manager modules plus the one remaining
// optional PowerShell dependency (shell.host: pwsh — see
// docs/plans/go-rewrite.md §11).
var checkedCommands = []string{
	"winget", "choco", "pipx", "npm", "cargo", "go", "gem", "eget", "pwsh",
}

func newDoctorCommand() *cobra.Command {
	return &cobra.Command{
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
			return nil
		},
	}
}
