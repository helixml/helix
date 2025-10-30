package moonlight

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "moonlight",
	Short:   "Moonlight client pairing management",
	Aliases: []string{"ml", "pair"},
	Long:    `Manage Moonlight client pairing for Wolf streaming`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listPendingCmd.RunE(cmd, args)
	},
}

func New() *cobra.Command {
	rootCmd.AddCommand(listPendingCmd)
	rootCmd.AddCommand(pairCmd)
	return rootCmd
}