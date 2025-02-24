package organization

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "organization",
	Short:   "Manage organizations",
	Long:    `Commands for managing organizations in Helix.`,
	Aliases: []string{"orgs"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the list command
		return listCmd.RunE(cmd, args)
	},
}

// GetRootCmd returns the root command for organizations
func New() *cobra.Command {
	return rootCmd
}
