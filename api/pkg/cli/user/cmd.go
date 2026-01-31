package user

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "user",
	Short:   "Manage users",
	Long:    `Commands for managing users in Helix.`,
	Aliases: []string{"users"},
}

// New returns the root command for users
func New() *cobra.Command {
	return rootCmd
}
