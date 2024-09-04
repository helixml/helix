package helix

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/dataprep/qapairs"
	"github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/spf13/cobra"
)

var prompt []string
var theText []string
var qaPairGenModel string // model to use

func newQapairCommand() *cobra.Command {
	var qapairCmd = &cobra.Command{
		Use:   "qapairs",
		Short: "A CLI tool for running QA pair commands",
		RunE: func(cmd *cobra.Command, args []string) error {
			serverConfig, err := config.LoadServerConfig()
			if err != nil {
				return fmt.Errorf("failed to load server config: %v", err)
			}
			ps, err := pubsub.New(serverConfig.PubSub.StoreDir)
			if err != nil {
				return err
			}
			helixInference := openai.NewInternalHelixServer(&serverConfig, ps)

			client, err := createDataPrepOpenAIClient(&serverConfig, helixInference)
			if err != nil {
				return err
			}

			if qaPairGenModel != "" {
				serverConfig.FineTuning.QAPairGenModel = qaPairGenModel
			}

			return qapairs.Run(client, "n/a", "n/a", serverConfig.FineTuning.QAPairGenModel, prompt, theText)
		},
	}

	qapairCmd.Flags().StringVar(&qaPairGenModel, "model", "",
		"Model to use if you want to override default",
	)
	qapairCmd.Flags().StringSliceVar(&prompt, "prompt", []string{},
		"Prompt(s) to use, defaults to all",
	)
	qapairCmd.Flags().StringSliceVar(&theText, "text", []string{},
		"Text(s) to use, defaults to all",
	)
	return qapairCmd
}
