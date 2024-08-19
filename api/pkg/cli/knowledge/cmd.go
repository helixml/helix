package knowledge

import "github.com/spf13/cobra"

var rootCmd = &cobra.Command{
	Use:   "knowledge",
	Short: "Helix knowledge management",
	Long:  `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}
