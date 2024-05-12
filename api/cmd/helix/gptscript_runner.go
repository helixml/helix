package helix

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/gptscript"
)

func newGptScriptRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "gptscript-runner",
		Short:   "Start the helix gptscript runner.",
		Long:    "Start the helix gptscript runner.",
		Example: "TBD",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return gptScriptRunner(cmd)
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

	runner := gptscript.NewRunner(&cfg)

	ctx, cancel := context.WithCancel(context.Background())

	stopSigCh := make(chan os.Signal, 1)
	signal.Notify(stopSigCh, syscall.SIGQUIT, syscall.SIGTERM, os.Interrupt)

	go func() {
		log.Info().Msg("received stop signal, stopping")
		<-stopSigCh
		cancel()
	}()

	return runner.Run(ctx)
}
