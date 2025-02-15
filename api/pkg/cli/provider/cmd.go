package provider

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage provider endpoints",
	Long:  `Commands for managing provider endpoints in Helix.`,
}

// GetRootCmd returns the root command for provider endpoints
func New() *cobra.Command {
	return rootCmd
}
