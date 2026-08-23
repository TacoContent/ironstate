// Package cli wires the cobra command tree. Business logic (loading,
// flattening, dispatch) lives in other internal packages — commands here
// only parse flags, load Config, and call into them.
package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/config"
	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/facts"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/handlers"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/pathutil"
	"github.com/TacoContent/ironstate/internal/tasks"
	"github.com/TacoContent/ironstate/internal/template"
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

	resolvedFile := pathutil.ResolveUserPath(cfg.File)
	doc, err := packages.LoadHierarchy(resolvedFile)
	if err != nil {
		return NewLoadError(err)
	}
	docMap := model.AsMap(doc)

	hostFacts := facts.Gather()
	vars := model.Vars(docMap)
	fset := filters.New()

	// Whole-document '-Soft' pass: resolves facts/vars now, leaves any
	// bare id/fact reference untouched for the per-leaf strict pass in
	// internal/engine, once that registry actually exists.
	softCtx := map[string]any{"facts": hostFacts, "vars": vars}
	if err := template.ResolveInPlace(docMap, softCtx, fset, "site", true); err != nil {
		return NewLoadError(err)
	}

	taskList, err := model.TaskList(docMap)
	if err != nil {
		return NewLoadError(err)
	}

	root := filepath.Dir(resolvedFile)
	leaves, err := tasks.Expand(taskList, tasks.Options{
		ModuleNames:  handlers.AllModuleNames,
		PackagesRoot: root,
		Facts:        hostFacts,
		Vars:         vars,
		Filters:      fset,
	})
	if err != nil {
		return NewLoadError(err)
	}

	filtered := engine.FilterByTags(leaves, cfg.Tags)

	results, stopped, err := engine.Run(filtered, engine.Options{
		Handlers: handlers.All(),
		Facts:    hostFacts,
		Vars:     vars,
		Filters:  fset,
		Apply:    cfg.Apply,
	})
	if err != nil {
		return NewRunError(err)
	}

	if cfg.Output == "json" {
		if err := engine.PrintJSON(cmd.OutOrStdout(), results); err != nil {
			return NewRunError(err)
		}
	} else {
		if err := engine.PrintTable(cmd.OutOrStdout(), results); err != nil {
			return NewRunError(err)
		}
	}

	if stopped {
		return NewRunError(fmt.Errorf("run stopped due to a failed task"))
	}
	return nil
}
