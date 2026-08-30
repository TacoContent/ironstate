// Package cli wires the cobra command tree. Business logic (loading,
// flattening, dispatch) lives in other internal packages — commands here
// only parse flags, load Config, and call into them.
package cli

import (
	"fmt"
	"path/filepath"
	"time"

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
	"github.com/TacoContent/ironstate/internal/ui"
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
		Short:         "Declarative, Ansible-style task runner driven by main.yml",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runApply,
	}

	flags := cmd.Flags()
	flags.String("playbook", "", "path to the site/main YAML document, or a directory/bare name to search for one (site.yml, main.yml, etc.); defaults to the current directory")
	flags.StringArray("vars-file", nil, "additional vars document to merge in on top of the auto-detected hostname/architecture/os_family/platform chain, before --var (repeatable; later files win on overlapping keys)")
	flags.StringArray("var", nil, "override a var by dotted key path: --var key=value (repeatable, highest precedence)")
	flags.Bool("apply", false, "actually apply changes (default: dry-run)")
	flags.StringSlice("tags", nil, "restrict processing to tasks/actions carrying any of these tags")
	flags.String("output", "table", "result output format: table|json")
	flags.BoolP("verbose", "v", false, "verbose output")
	flags.Bool("no-color", false, "disable colored output")

	cmd.AddCommand(newVersionCommand())
	cmd.AddCommand(newFiltersCommand())
	cmd.AddCommand(newDoctorCommand())
	cmd.AddCommand(newInitCommand())

	return cmd, nil
}

func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	cfg, err := config.Load(cmd.Flags())
	if err != nil {
		return nil, NewLoadError(err)
	}
	return cfg, nil
}

func runApply(cmd *cobra.Command, _ []string) error {
	cfg, err := loadConfig(cmd)
	if err != nil {
		return err
	}
	if noColor, _ := cmd.Flags().GetBool("no-color"); noColor {
		ui.Enabled = false
	}
	tableOutput := cfg.Output != "json"

	// Loaded from the current working directory (not the playbook's own
	// directory, and not relative to the binary) - matches how
	// config.Load's own ironstate.yaml is resolved, and how these were
	// found in the original apply.sh/ironstate.ps1 (always run from a
	// repo/project root). A missing file is not an error. Must happen
	// before anything that could reference an env var (facts, lookup('env',
	// ...), '.env'-derived vars/tags) - loaded first, deliberately.
	if _, err := packages.ImportEnvFile(".env"); err != nil {
		return NewLoadError(err)
	}
	if err := packages.RegisterEnvFileSecrets(".secrets"); err != nil {
		return NewLoadError(err)
	}

	progress := newProgressReporter()
	progress.Start()
	defer progress.Stop()

	// engine.Info/Warn/Danger print leaf-by-leaf lines straight to
	// stdout/stderr, with no knowledge of the spinner above still
	// animating its own line - left alone, the two interleave into
	// garbled output (e.g. "◒ loading playbook hierarchy───...",
	// a spinner frame immediately followed by unrelated table/log text
	// with no newline between them). Route every such print through
	// progress.Pause for as long as this run's spinner exists, restoring
	// the real functions on return - mirrors wait_for's own spinner pause
	// (internal/handlers/util.go's pauseSpinnerForPrint), just scoped to
	// the whole run instead of one task.
	origInfo, origWarn, origDanger := engine.Info, engine.Warn, engine.Danger
	origPackagesWarn, origTasksWarn := packages.Warn, tasks.Warn
	engine.Info = func(format string, args ...any) { progress.Pause(func() { origInfo(format, args...) }) }
	engine.Warn = func(format string, args ...any) { progress.Pause(func() { origWarn(format, args...) }) }
	engine.Danger = func(format string, args ...any) { progress.Pause(func() { origDanger(format, args...) }) }
	packages.Warn = func(format string, args ...any) { progress.Pause(func() { origPackagesWarn(format, args...) }) }
	tasks.Warn = func(format string, args ...any) { progress.Pause(func() { origTasksWarn(format, args...) }) }
	defer func() {
		engine.Info, engine.Warn, engine.Danger = origInfo, origWarn, origDanger
		packages.Warn, tasks.Warn = origPackagesWarn, origTasksWarn
	}()

	progress.Message("loading playbook inputs")

	resolvedFile, err := packages.ResolvePlaybookPath(cfg.Playbook)
	if err != nil {
		return NewLoadError(err)
	}

	progress.Message("gathering host facts")
	hostFacts := facts.Gather()
	progress.Message("loading playbook hierarchy")
	doc, err := packages.LoadHierarchy(resolvedFile, hostFacts)
	if err != nil {
		return NewLoadError(err)
	}
	docMap := model.AsMap(doc)
	repoRoot := filepath.Dir(resolvedFile)

	// --vars-file is repeatable: each extra vars document merges in, in
	// the order given, after the auto-detected hostname/architecture/
	// os_family/platform chain (and the legacy per-username overlay) so
	// they win on overlapping keys - a later --vars-file wins over an
	// earlier one - but all before --var (the most explicit override of
	// all).
	for _, raw := range cfg.VarsFiles {
		progress.Message("merging vars files")
		varsFilePath := pathutil.ResolveUserPath(raw)
		overlay, err := packages.LoadFile(varsFilePath, repoRoot)
		if err != nil {
			return NewLoadError(err)
		}
		overlayMap, ok := overlay.(map[string]any)
		if !ok {
			return NewLoadError(fmt.Errorf("--vars-file %s must use the explicit 'tasks:'/'vars:' mapping form", varsFilePath))
		}
		docMap = packages.MergeDocuments(docMap, overlayMap)
	}

	progress.Message("preparing playbook variables")
	vars := model.Vars(docMap)
	for _, raw := range cfg.VarOverrides {
		key, value, err := model.ParseVarOverride(raw)
		if err != nil {
			return NewLoadError(err)
		}
		model.SetVarPath(vars, key, model.CoerceVarValue(value))
	}
	docMap["vars"] = vars

	engine.RegisterSecretVarPaths(vars)
	fset := filters.New()

	interpreters := cfg.FilterInterpreters
	if len(interpreters) == 0 {
		interpreters = filters.DefaultInterpreters()
	}
	filtersDir := cfg.FiltersDir
	if !filepath.IsAbs(filtersDir) {
		filtersDir = filepath.Join(repoRoot, filtersDir)
	}
	scriptPool, _, err := filters.DiscoverScriptFilters(fset, filtersDir, interpreters)
	if err != nil {
		return NewLoadError(err)
	}
	defer func() { _ = scriptPool.Close() }()

	// Whole-document '-Soft' pass: resolves facts/vars now, leaves any
	// bare id/fact reference untouched for the per-leaf strict pass in
	// internal/engine, once that registry actually exists.
	softCtx := map[string]any{"facts": hostFacts, "vars": vars}
	if err := template.ResolveInPlace(docMap, softCtx, fset, "site", true); err != nil {
		return NewLoadError(err)
	}

	progress.Message("expanding playbook tasks")
	taskList, err := model.TaskList(docMap)
	if err != nil {
		return NewLoadError(err)
	}

	root := repoRoot
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
	progress.Message("running playbook tasks")

	// The facts table waits for every 'fact'/'mount_facts' leaf to have
	// actually run (not just the fixed set gathered up front) before
	// printing, so it shows the complete, final set of facts - see
	// engine.Options.OnFactsGathered's doc comment.
	var factsErr error
	start := time.Now()
	results, stopped, err := engine.Run(filtered, engine.Options{
		Handlers: handlers.All(),
		Facts:    hostFacts,
		Vars:     vars,
		Filters:  fset,
		Apply:    cfg.Apply,
		Verbose:  cfg.Verbose,
		Progress: func(stage, detail string, index, total int) {
			progress.Step(stage, index, total, detail)
		},
		OnFactsGathered: func(allFacts map[string]any) {
			if !tableOutput {
				return
			}
			progress.Pause(func() { factsErr = ui.PrintFacts(cmd.OutOrStdout(), allFacts) })
		},
	})
	elapsed := time.Since(start)
	if err != nil {
		return NewRunError(err)
	}
	if factsErr != nil {
		return NewRunError(factsErr)
	}

	progress.Stop()

	if cfg.Output == "json" {
		if err := engine.PrintJSON(cmd.OutOrStdout(), results); err != nil {
			return NewRunError(err)
		}
	} else {
		if err := engine.PrintTable(cmd.OutOrStdout(), results); err != nil {
			return NewRunError(err)
		}
		if err := engine.PrintSummary(cmd.OutOrStdout(), engine.ComputeStats(results), elapsed); err != nil {
			return NewRunError(err)
		}
	}

	if stopped {
		return NewRunError(fmt.Errorf("run stopped due to a failed task"))
	}
	return nil
}
