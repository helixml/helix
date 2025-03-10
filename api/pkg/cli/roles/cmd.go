package roles

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "roles",
	Short:   "Manage roles",
	Long:    `Commands for managing roles in Helix.`,
	Aliases: []string{"roles"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the list command
		return listCmd.RunE(cmd, args)
	},
}

// GetRootCmd returns the root command for organizations
func New() *cobra.Command {
	return rootCmd
}
