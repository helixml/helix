package helix

import (
	"github.com/helixml/helix/api/pkg/evals"
	"github.com/spf13/cobra"
)

var evalTargets []string

func newEvalsCommand() *cobra.Command {
	var evalsCmd = &cobra.Command{
		Use:   "evals",
		Short: "A CLI tool for evaluating finetuned LLMs",
		Run: func(cmd *cobra.Command, args []string) {
			evals.Run()
		},
	}
	evalsCmd.Flags().StringSliceVar(&evalTargets, "target", []string{},
		"Target(s) to use, defaults to all",
	)

	return evalsCmd
}
