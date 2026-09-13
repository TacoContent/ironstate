package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/engine"
	"github.com/TacoContent/ironstate/internal/filters"
	"github.com/TacoContent/ironstate/internal/model"
	"github.com/TacoContent/ironstate/internal/pluginhost"
)

type pluginTestClient interface {
	Handlers() map[string]engine.Handler
	Close() error
}

var pluginTestStore = pluginhost.DefaultStore

var pluginTestLaunch = func(command *exec.Cmd) (pluginTestClient, error) {
	return pluginhost.LaunchWithCallbacks(command, filters.New())
}

func newPluginCommand() *cobra.Command {
	command := &cobra.Command{Use: "plugin", Short: "Manage external handler plugins"}
	command.AddCommand(newPluginInstallCommand(), newPluginListCommand(), newPluginInfoCommand(), newPluginUpdateCommand(), newPluginUninstallCommand(), newPluginTestCommand(), newPluginBenchCommand())
	return command
}

func newPluginBenchCommand() *cobra.Command {
	var handlerName, rawItem, operation string
	var iterations int
	var apply bool
	command := &cobra.Command{
		Use:   "bench <organization.plugin>",
		Short: "Benchmark an installed external handler",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if iterations < 1 {
				return fmt.Errorf("iterations must be at least 1")
			}
			if err := validatePluginBenchOperation(operation); err != nil {
				return err
			}
			namespace, _, err := parsePluginReference(args[0])
			if err != nil {
				return err
			}
			item, err := parsePluginTestItem(rawItem)
			if err != nil {
				return err
			}
			store, err := pluginTestStore()
			if err != nil {
				return err
			}
			manifest, err := store.ResolveVersion(namespace, "latest")
			if err != nil {
				return fmt.Errorf("plugin %s is not installed; run: ironstate plugin install %s@latest", namespace, namespace)
			}
			binary, err := store.BinaryPath(namespace, manifest.Version)
			if err != nil {
				return err
			}
			client, err := pluginTestLaunch(exec.Command(binary)) //nolint:gosec // path comes from a validated per-user plugin manifest
			if err != nil {
				return fmt.Errorf("launch plugin %s@%s: %w", namespace, manifest.Version, err)
			}
			defer func() { _ = client.Close() }()
			handler, ok := client.Handlers()[handlerName]
			if !ok {
				return fmt.Errorf("handler %q is not declared by plugin %s@%s", handlerName, namespace, manifest.Version)
			}
			return runPluginBench(command, handler, handlerName, item, operation, iterations, apply)
		},
	}
	command.Flags().StringVar(&handlerName, "handler", "", "plugin handler name")
	command.Flags().StringVar(&rawItem, "item", "", "handler item as a YAML or JSON mapping")
	command.Flags().StringVar(&operation, "operation", "test", "operation to benchmark: test|describe|install|uninstall")
	command.Flags().IntVar(&iterations, "iterations", 10, "number of operation calls")
	command.Flags().BoolVar(&apply, "apply", false, "allow install or uninstall operations to make changes")
	_ = command.MarkFlagRequired("handler")
	_ = command.MarkFlagRequired("item")
	return command
}

func validatePluginBenchOperation(operation string) error {
	switch strings.ToLower(operation) {
	case "test", "describe", "install", "uninstall":
		return nil
	default:
		return fmt.Errorf("unsupported benchmark operation %q; choose test, describe, install, or uninstall", operation)
	}
}

type pluginBenchResult struct {
	Handler    string  `json:"handler"`
	Operation  string  `json:"operation"`
	Iterations int     `json:"iterations"`
	Apply      bool    `json:"apply"`
	TotalNS    int64   `json:"total_ns"`
	TotalMS    float64 `json:"total_ms"`
	AverageNS  float64 `json:"average_ns"`
	AverageMS  float64 `json:"average_ms"`
	MinNS      int64   `json:"min_ns"`
	MaxNS      int64   `json:"max_ns"`
}

func runPluginBench(command *cobra.Command, handler engine.Handler, handlerName string, item map[string]any, operation string, iterations int, apply bool) error {
	ctx := engine.Context{Flat: map[string]any{}, Apply: apply}
	durations := make([]time.Duration, 0, iterations)
	for i := 0; i < iterations; i++ {
		started := time.Now()
		var err error
		switch strings.ToLower(operation) {
		case "test":
			_, err = handler.Test(item, "", ctx)
		case "describe":
			_, err = handler.Describe(item, engine.ActionInstall, ctx)
		case "install":
			_, err = handler.Install(item, "", ctx)
		case "uninstall":
			_, err = handler.Uninstall(item, "", ctx)
		}
		if err != nil {
			return fmt.Errorf("benchmark %s iteration %d: %w", operation, i+1, err)
		}
		durations = append(durations, time.Since(started))
	}
	var total time.Duration
	min, max := durations[0], durations[0]
	for _, duration := range durations {
		total += duration
		if duration < min {
			min = duration
		}
		if duration > max {
			max = duration
		}
	}
	result := pluginBenchResult{
		Handler: handlerName, Operation: strings.ToLower(operation), Iterations: iterations, Apply: apply,
		TotalNS: total.Nanoseconds(), TotalMS: float64(total) / float64(time.Millisecond),
		AverageNS: float64(total.Nanoseconds()) / float64(iterations), AverageMS: float64(total) / float64(iterations) / float64(time.Millisecond),
		MinNS: min.Nanoseconds(), MaxNS: max.Nanoseconds(),
	}
	return json.NewEncoder(command.OutOrStdout()).Encode(result)
}

func newPluginInstallCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "install <organization.plugin>[@version]",
		Short: "Install an external handler plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			store, err := pluginhost.DefaultStore()
			if err != nil {
				return err
			}
			namespace, version, err := parsePluginReference(args[0])
			if err != nil {
				return err
			}
			manifest, err := pluginhost.Install(command.Context(), store, namespace, version)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "installed %s@%s\n", manifest.Namespace(), manifest.Version)
			return err
		},
	}
}

func newPluginListCommand() *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List installed external handler plugins", Args: cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			store, err := pluginhost.DefaultStore()
			if err != nil {
				return err
			}
			manifests, err := store.ListInstalled()
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(command.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "PLUGIN\tVERSION\tHANDLERS\tSUMMARY"); err != nil {
				return err
			}
			for _, manifest := range manifests {
				if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", manifest.Namespace(), manifest.Version, strings.Join(manifest.HandlerNames, ","), manifest.DocSummary); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	}
}

func newPluginInfoCommand() *cobra.Command {
	return &cobra.Command{
		Use: "info <organization.plugin>", Short: "Show installed plugin manifests", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if err := validatePluginNamespace(args[0]); err != nil {
				return err
			}
			store, err := pluginhost.DefaultStore()
			if err != nil {
				return err
			}
			manifests, err := store.ListInstalled()
			if err != nil {
				return err
			}
			found := false
			for _, manifest := range manifests {
				if manifest.Namespace() != args[0] {
					continue
				}
				found = true
				if _, err := fmt.Fprintf(command.OutOrStdout(), "%s@%s\n  source: %s\n  checksum: %s\n  license: %s\n  handlers: %s\n  summary: %s\n", manifest.Namespace(), manifest.Version, manifest.Source, manifest.Checksum, manifest.License, strings.Join(manifest.HandlerNames, ", "), manifest.DocSummary); err != nil {
					return err
				}
			}
			if !found {
				return fmt.Errorf("plugin %s is not installed", args[0])
			}
			return nil
		},
	}
}

func newPluginUpdateCommand() *cobra.Command {
	var all bool
	command := &cobra.Command{
		Use: "update <organization.plugin>[@version]", Short: "Install an updated plugin version", Args: cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			if all == (len(args) == 1) {
				return fmt.Errorf("specify one plugin or --all")
			}
			store, err := pluginhost.DefaultStore()
			if err != nil {
				return err
			}
			references := args
			if all {
				manifests, err := store.ListInstalled()
				if err != nil {
					return err
				}
				seen := map[string]bool{}
				for _, manifest := range manifests {
					if !seen[manifest.Namespace()] {
						references = append(references, manifest.Namespace())
						seen[manifest.Namespace()] = true
					}
				}
			}
			for _, reference := range references {
				namespace, version, err := parsePluginReference(reference)
				if err != nil {
					return err
				}
				manifest, err := pluginhost.Install(command.Context(), store, namespace, version)
				if err != nil {
					return err
				}
				if _, err := fmt.Fprintf(command.OutOrStdout(), "updated %s@%s\n", manifest.Namespace(), manifest.Version); err != nil {
					return err
				}
			}
			return nil
		},
	}
	command.Flags().BoolVar(&all, "all", false, "update every installed plugin")
	return command
}

func newPluginUninstallCommand() *cobra.Command {
	return &cobra.Command{
		Use: "uninstall <organization.plugin>[@version]", Short: "Remove an installed plugin", Args: cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			namespace, version, err := parsePluginReference(args[0])
			if err != nil {
				return err
			}
			store, err := pluginhost.DefaultStore()
			if err != nil {
				return err
			}
			if strings.Contains(args[0], "@") {
				if err := store.RemoveVersion(namespace, version); err != nil {
					return err
				}
			} else {
				manifests, err := store.ListInstalled()
				if err != nil {
					return err
				}
				for _, manifest := range manifests {
					if manifest.Namespace() == namespace {
						if err := store.RemoveVersion(namespace, manifest.Version); err != nil {
							return err
						}
					}
				}
			}
			_, err = fmt.Fprintf(command.OutOrStdout(), "uninstalled %s\n", namespace)
			return err
		},
	}
}

func newPluginTestCommand() *cobra.Command {
	var handlerName, rawItem string
	var apply bool
	command := &cobra.Command{
		Use:   "test <organization.plugin>",
		Short: "Test an installed external handler without a playbook",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			namespace, _, err := parsePluginReference(args[0])
			if err != nil {
				return err
			}
			item, err := parsePluginTestItem(rawItem)
			if err != nil {
				return err
			}
			store, err := pluginTestStore()
			if err != nil {
				return err
			}
			manifest, err := store.ResolveVersion(namespace, "latest")
			if err != nil {
				return fmt.Errorf("plugin %s is not installed; run: ironstate plugin install %s@latest", namespace, namespace)
			}
			binary, err := store.BinaryPath(namespace, manifest.Version)
			if err != nil {
				return err
			}
			client, err := pluginTestLaunch(exec.Command(binary)) //nolint:gosec // path comes from a validated per-user plugin manifest
			if err != nil {
				return fmt.Errorf("launch plugin %s@%s: %w", namespace, manifest.Version, err)
			}
			defer func() { _ = client.Close() }()
			handler, ok := client.Handlers()[handlerName]
			if !ok {
				return fmt.Errorf("handler %q is not declared by plugin %s@%s", handlerName, namespace, manifest.Version)
			}
			return runPluginTest(command, handler, handlerName, item, apply)
		},
	}
	command.Flags().StringVar(&handlerName, "handler", "", "plugin handler name")
	command.Flags().StringVar(&rawItem, "item", "", "handler item as a YAML or JSON mapping")
	command.Flags().BoolVar(&apply, "apply", false, "run the install or uninstall operation after testing")
	_ = command.MarkFlagRequired("handler")
	_ = command.MarkFlagRequired("item")
	return command
}

func parsePluginTestItem(raw string) (map[string]any, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("plugin test item is required")
	}
	var value any
	var err error
	if strings.HasPrefix(trimmed, "{") {
		if err = json.Unmarshal([]byte(trimmed), &value); err != nil {
			return nil, fmt.Errorf("parse plugin test JSON item: %w", err)
		}
	} else {
		value, err = model.Unmarshal([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("parse plugin test YAML item: %w", err)
		}
	}
	item, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("plugin test item must be a YAML or JSON mapping")
	}
	return item, nil
}

func runPluginTest(command *cobra.Command, handler engine.Handler, handlerName string, item map[string]any, apply bool) error {
	ctx := engine.Context{Flat: map[string]any{}, Apply: apply}
	satisfied, err := handler.Test(item, "", ctx)
	if err != nil {
		return err
	}
	action := engine.ActionInstall
	if state, _ := item["state"].(string); strings.EqualFold(state, "absent") || strings.EqualFold(state, "removed") {
		action = engine.ActionUninstall
	}
	description, err := handler.Describe(item, action, ctx)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(command.ErrOrStderr(), "%s\n", description); err != nil {
		return err
	}
	result := engine.ExecResult{}
	if apply && !satisfied {
		if action == engine.ActionUninstall {
			result, err = handler.Uninstall(item, "", ctx)
		} else {
			result, err = handler.Install(item, "", ctx)
		}
		if err != nil {
			return err
		}
	}
	return json.NewEncoder(command.OutOrStdout()).Encode(pluginTestResult{Handler: handlerName, Satisfied: satisfied, Action: action, Apply: apply, Exec: pluginTestExecResult{
		RC: result.RC, Stdout: result.Stdout, StdoutLines: result.StdoutLines,
		Stderr: result.Stderr, StderrLines: result.StderrLines, Extra: result.Extra,
	}})
}

type pluginTestResult struct {
	Handler   string               `json:"handler"`
	Satisfied bool                 `json:"satisfied"`
	Action    engine.Action        `json:"action"`
	Apply     bool                 `json:"apply"`
	Exec      pluginTestExecResult `json:"exec"`
}

type pluginTestExecResult struct {
	RC          int            `json:"rc"`
	Stdout      string         `json:"stdout"`
	StdoutLines []string       `json:"stdout_lines"`
	Stderr      string         `json:"stderr"`
	StderrLines []string       `json:"stderr_lines"`
	Extra       map[string]any `json:"extra,omitempty"`
}

func parsePluginReference(value string) (string, string, error) {
	parts := strings.Split(value, "@")
	if len(parts) > 2 || len(parts) == 0 || parts[0] == "" {
		return "", "", fmt.Errorf("plugin %q must use organization.plugin[@version]", value)
	}
	if err := validatePluginNamespace(parts[0]); err != nil {
		return "", "", err
	}
	if len(parts) == 1 {
		return parts[0], "latest", nil
	}
	if parts[1] == "" {
		return "", "", fmt.Errorf("plugin %q has an empty version", value)
	}
	return parts[0], parts[1], nil
}

func validatePluginNamespace(namespace string) error {
	parts := strings.Split(namespace, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(namespace, `\\/`) {
		return fmt.Errorf("plugin %q must use organization.plugin form", namespace)
	}
	return nil
}

func installDeclaredPlugin(store *pluginhost.Store, namespace, version string) (pluginhost.Manifest, error) {
	return pluginhost.Install(context.Background(), store, namespace, version)
}
