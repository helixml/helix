package model

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "model",
	Short:   "Helix model management",
	Aliases: []string{"models", "m"},
	Long:    `Manage Helix models including Ollama and VLLM models. Supports listing, creating, updating, deleting, and inspecting model configurations.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listCmd.RunE(cmd, args)
	},
}

func New() *cobra.Command {
	return rootCmd
}
