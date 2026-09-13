package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/facts"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/pluginhost"
)

// checkedCommands are looked up on PATH by `ironstate doctor`. This list
// mirrors ironstate.ps1's package-manager modules plus the one remaining
// optional PowerShell dependency (shell.host: pwsh — see
// docs/plans/go-rewrite.md §11).
var checkedCommands = []string{
	"winget", "choco", "brew", "apt-get", "pipx", "npm", "cargo", "go", "gem", "eget", "xget", "pwsh",
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

			playbook, err := cmd.Flags().GetString("playbook")
			if err != nil {
				return err
			}
			if playbook != "" {
				if err := doctorCheckPlaybook(cmd, playbook); err != nil {
					return err
				}
			}
			return nil
		},
	}
	cmd.Flags().String("filters-dir", "filters", "directory to scan for external script filters")
	cmd.Flags().String("playbook", "", "path to a playbook whose external plugins should be validated")
	return cmd
}

func doctorCheckPlaybook(cmd *cobra.Command, playbook string) error {
	resolvedFile, err := packages.ResolvePlaybookPath(playbook)
	if err != nil {
		return NewLoadError(err)
	}
	doc, err := packages.LoadHierarchy(resolvedFile, facts.Gather())
	if err != nil {
		return NewLoadError(err)
	}
	docMap := model.AsMap(doc)
	declared, err := model.Plugins(docMap)
	if err != nil {
		return NewLoadError(err)
	}
	taskList, err := model.TaskList(docMap)
	if err != nil {
		return NewLoadError(err)
	}

	cmd.Printf("\nplugin validation for %s:\n", resolvedFile)
	resolution, problems := doctorResolvePlugins(declared, filepath.Dir(resolvedFile))
	for _, problem := range problems {
		cmd.Printf("[plugin]  %s\n", problem)
	}
	for _, module := range qualifiedModules(taskList) {
		if _, ok := resolution.handlers[module]; !ok {
			parts := strings.Split(module, ".")
			namespace := strings.Join(parts[:2], ".")
			if declaredPlugin(namespace, declared) && resolution.plugins[namespace] {
				problems = append(problems, fmt.Sprintf("%s is not declared by plugin %s; run: ironstate plugin update %s", module, namespace, namespace))
			} else {
				problems = append(problems, fmt.Sprintf("%s is used but plugin %s is not declared; add plugins: - use: %s@<version>, then run: ironstate plugin install %s@<version>", module, namespace, namespace, namespace))
			}
			cmd.Printf("[plugin]  %s\n", problems[len(problems)-1])
		}
	}
	if len(problems) > 0 {
		return NewRunError(fmt.Errorf("plugin validation found %d problem(s)", len(problems)))
	}
	cmd.Println("[ok]      declared plugins resolve, handshake, and provide every qualified handler in use")
	return nil
}

type doctorPluginResolution struct {
	handlers map[string]bool
	plugins  map[string]bool
}

func doctorResolvePlugins(declared []model.Plugin, root string) (doctorPluginResolution, []string) {
	resolution := doctorPluginResolution{handlers: map[string]bool{}, plugins: map[string]bool{}}
	store, err := pluginhost.DefaultStore()
	if err != nil {
		return resolution, []string{err.Error()}
	}
	lock, err := pluginhost.LoadLockFile(root)
	if err != nil {
		return resolution, []string{err.Error()}
	}
	return doctorResolveInstalledPlugins(store, lock, declared, pluginhost.Launch)
}

func doctorResolveInstalledPlugins(store *pluginhost.Store, lock *pluginhost.LockFile, declared []model.Plugin, launch func(*exec.Cmd) (*pluginhost.Client, error)) (doctorPluginResolution, []string) {
	resolution := doctorPluginResolution{handlers: map[string]bool{}, plugins: map[string]bool{}}
	var problems []string
	for _, plugin := range declared {
		version, locked, isLocked := doctorPluginVersion(plugin, lock)
		manifest, err := store.ResolveVersion(plugin.Namespace, version)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s@%s is not installed; run: ironstate plugin install %s@%s", plugin.Namespace, version, plugin.Namespace, version))
			continue
		}
		if isLocked {
			if err := pluginhost.VerifyChecksum(manifest, locked); err != nil {
				problems = append(problems, err.Error())
				continue
			}
		}
		binary, err := store.BinaryPath(plugin.Namespace, manifest.Version)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		client, err := launch(exec.Command(binary)) //nolint:gosec // path comes from a validated per-user plugin manifest
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s@%s failed to launch or complete the plugin handshake; run: ironstate plugin update %s (%v)", plugin.Namespace, manifest.Version, plugin.Namespace, err))
			continue
		}
		qualified, err := client.QualifiedHandlers(plugin.Namespace)
		_ = client.Close()
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		for name := range qualified {
			resolution.handlers[name] = true
		}
		resolution.plugins[plugin.Namespace] = true
	}
	return resolution, problems
}

func doctorPluginVersion(plugin model.Plugin, lock *pluginhost.LockFile) (string, pluginhost.LockedPlugin, bool) {
	if lock != nil {
		if locked, ok := lock.Plugins[plugin.Namespace]; ok {
			return locked.Version, locked, true
		}
	}
	return plugin.Version, pluginhost.LockedPlugin{}, false
}

func qualifiedModules(tasks []any) []string {
	seen := map[string]bool{}
	var visit func([]any)
	visit = func(items []any) {
		for _, raw := range items {
			item := model.AsMap(raw)
			for key := range item {
				if strings.Count(key, ".") == 2 {
					seen[key] = true
				}
			}
			visit(model.AsList(item["actions"]))
		}
	}
	visit(tasks)
	modules := make([]string, 0, len(seen))
	for module := range seen {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}

func declaredPlugin(namespace string, declared []model.Plugin) bool {
	for _, plugin := range declared {
		if plugin.Namespace == namespace {
			return true
		}
	}
	return false
}
