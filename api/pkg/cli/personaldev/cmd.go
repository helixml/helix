package personaldev

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "personaldev",
	Short:   "Personal development environments management",
	Aliases: []string{"pde", "dev"},
	Long:    `Manage personal development environments with Wolf streaming integration`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listCmd.RunE(cmd, args)
	},
}

func New() *cobra.Command {
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(createCmd)
	rootCmd.AddCommand(deleteCmd)
	rootCmd.AddCommand(startCmd)
	rootCmd.AddCommand(stopCmd)
	return rootCmd
}