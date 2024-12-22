package fs

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:     "filesystem",
	Short:   "Helix filesystem management",
	Aliases: []string{"fs"},
	Long:    `TODO`,
	Run: func(*cobra.Command, []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}
