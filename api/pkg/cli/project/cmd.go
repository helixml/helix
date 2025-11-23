package project

import (
	"github.com/spf13/cobra"
)

func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "project",
		Short:   "Project management commands",
		Aliases: []string{"proj", "p"},
	}

	cmd.AddCommand(newListSamplesCommand())
	cmd.AddCommand(newForkCommand())
	cmd.AddCommand(newListCommand())
	cmd.AddCommand(newInspectCommand())

	return cmd
}
