package helix

import (
	"github.com/helixml/helix/api/pkg/config"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func newGptScriptRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "gptscript-runner",
		Short:   "Start the helix gptscript runner.",
		Long:    "Start the helix gptscript runner.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gptscript(cmd)
		},
	}
	return runCmd
}

func gptScriptRunner(_ *cobra.Command) error {
	cfg, err := config.LoadGPTScriptRunnerConfig()
	if err != nil {
		log.Error().Err(err).Msg("failed to load gptscript runner config")
		return err
	}

	return nil
}
