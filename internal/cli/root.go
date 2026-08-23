// Package cli wires the cobra command tree. Business logic (loading,
// flattening, dispatch) lives in other internal packages — commands here
// only parse flags, load Config, and call into them.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/config"
)

// Execute builds and runs the root command.
func Execute() error {
	root, err := newRootCommand()
	if err != nil {
		return err
	}
	return root.Execute()
}

func newRootCommand() (*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:           "ironstate",
		Short:         "Declarative, Ansible-style task runner driven by site.yml",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runApply,
	}

	flags := cmd.Flags()
	flags.String("file", "site.yml", "path to the YAML task document")
	flags.String("packages-file", "", "deprecated alias for --file")
	flags.Bool("apply", false, "actually apply changes (default: dry-run)")
	flags.StringSlice("tags", nil, "restrict processing to tasks/actions carrying any of these tags")
	flags.String("output", "table", "result output format: table|json")
	flags.BoolP("verbose", "v", false, "verbose output")

	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newFiltersCommand())
	cmd.AddCommand(newDoctorCommand())

	return cmd, nil
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	packagesFile, _ := cmd.Flags().GetString("packages-file")
	cfg, err := config.Load(cmd.Flags())
	if err != nil {
		return nil, NewLoadError(err)
	}
	// --packages-file is a deprecated alias for --file; only honored if
	// --file was left at its default and --packages-file was actually set.
	if packagesFile != "" && !cmd.Flags().Changed("file") {
		cfg.File = packagesFile
	}
	return cfg, nil
}

func runApply(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	// The real load/flatten/dispatch pipeline lands in later phases
	// (internal/packages, internal/tasks, internal/engine — see
	// docs/plans/go-rewrite.md §10). For now this only proves flag/config
	// wiring end-to-end.
	cmd.Printf("ironstate: file=%s apply=%v tags=%v output=%s\n", cfg.File, cfg.Apply, cfg.Tags, cfg.Output)
	cmd.Println("engine not yet implemented (Phase 2+) — see docs/plans/go-rewrite.md")
	return nil
}
