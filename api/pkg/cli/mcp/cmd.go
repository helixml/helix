package mcp

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "mcp",
	Short:   "Helix mcp proxy",
	Aliases: []string{"m"},
	Long:    `TODO`,
	Run: func(*cobra.Command, []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}
