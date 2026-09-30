package cli

import (
	"github.com/spf13/cobra"

	"github.com/TacoContent/ironstate/internal/packages"
	"github.com/TacoContent/ironstate/internal/validation"
)

func newValidateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a playbook against ironstate.schema.json",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			playbook, err := cmd.Flags().GetString("playbook")
			if err != nil {
				return err
			}
			resolved, err := packages.ResolvePlaybookPath(playbook)
			if err != nil {
				return NewLoadError(err)
			}
			valid, err := validation.ValidatePlaybook(resolved)
			for _, file := range valid {
				cmd.Printf("%s: valid\n", file)
			}
			if err != nil {
				return err
			}
			return nil
		},
	}
	cmd.Flags().String("playbook", "", "path to the playbook file or directory to validate")
	if err := cmd.MarkFlagRequired("playbook"); err != nil {
		panic(err)
	}
	return cmd
}
