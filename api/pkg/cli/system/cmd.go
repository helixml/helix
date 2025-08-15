package system

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:     "system",
	Short:   "Manage Helix system configuration",
	Aliases: []string{"sys"},
	Long:    `Provides commands for managing system-wide Helix configuration, including global settings and administrative functions.`,
}

func New() *cobra.Command {
	return rootCmd
}
