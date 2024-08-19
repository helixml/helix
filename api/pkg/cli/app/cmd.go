package app

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:     "app",
	Short:   "Helix app management",
	Aliases: []string{"a"},
	Long:    `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}
