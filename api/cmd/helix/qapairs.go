package helix

import (
	"fmt"
	"os"

	"github.com/helixml/helix/api/pkg/dataprep/qapairs"
	"github.com/spf13/cobra"
)

var target []string
var prompt []string
var theText []string

func newQapairCommand() *cobra.Command {
	var qapairCmd = &cobra.Command{
		Use:   "qapairs",
		Short: "A CLI tool for running QA pair commands",
		Run: func(cmd *cobra.Command, args []string) {
			qapairs.Run(target, prompt, theText)
		},
	}

	qapairCmd.Flags().StringSliceVar(&target, "target", []string{},
		"Target(s) to use, defaults to all",
	)
	qapairCmd.Flags().StringSliceVar(&prompt, "prompt", []string{},
		"Prompt(s) to use, defaults to all",
	)
	qapairCmd.Flags().StringSliceVar(&theText, "text", []string{},
		"Text(s) to use, defaults to all",
	)

	if err := qapairCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	return qapairCmd
}
