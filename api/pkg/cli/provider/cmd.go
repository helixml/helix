package provider

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage provider endpoints",
	Long:  `Commands for managing provider endpoints in Helix.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the list command
		return listCmd.RunE(cmd, args)
	},
}

// GetRootCmd returns the root command for provider endpoints
func New() *cobra.Command {
	return rootCmd
}
