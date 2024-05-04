package helix

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			serverConfig, err := config.LoadServerConfig()
			if err != nil {
				return fmt.Errorf("failed to load server config: %v", err)
			}

			client, err := qapairs.NewClient(&serverConfig, nil, nil)
			if err != nil {
				return fmt.Errorf("failed to create client: %v", err)
			}

			return qapairs.Run(client, serverConfig.FineTuning.QAPairGenModel, prompt, theText)
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
	return qapairCmd
}
