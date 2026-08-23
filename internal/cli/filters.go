package cli

import (
	"sort"

	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/filters"
)

func newFiltersCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "filters",
		Short: "Inspect available template/expression filters",
	}
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List built-in and discovered script filters",
		RunE: func(cmd *cobra.Command, _ []string) error {
			dir, _ := cmd.Flags().GetString("dir")

			r := filters.New()
			pool, discovered, err := filters.DiscoverScriptFilters(r, dir, filters.DefaultInterpreters())
			if err != nil {
				return err
			}
			defer func() { _ = pool.Close() }()

			discoveredSet := make(map[string]bool, len(discovered))
			for _, name := range discovered {
				discoveredSet[name] = true
			}

			names := r.Names()
			sort.Strings(names)
			for _, name := range names {
				kind := "built-in"
				if discoveredSet[name] {
					kind = "script"
				}
				cmd.Printf("%-20s %s\n", name, kind)
			}
			return nil
		},
	}
	listCmd.Flags().String("dir", "filters", "directory to scan for external script filters")
	cmd.AddCommand(listCmd)
	return cmd
}
