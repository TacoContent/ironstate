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
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List built-in and discovered script filters",
		RunE: func(cmd *cobra.Command, _ []string) error {
			names := filters.Names()
			sort.Strings(names)
			for _, name := range names {
				cmd.Println(name)
			}
			return nil
		},
	})
	return cmd
}
