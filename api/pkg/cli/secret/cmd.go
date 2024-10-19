package secret

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "secret",
	Short:   "Helix secret management",
	Aliases: []string{"s"},
	Long:    `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}
