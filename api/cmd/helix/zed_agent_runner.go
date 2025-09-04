package helix

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/config"
)

func NewZedAgentRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "zed-agent-runner",
		Short:   "Start the Helix Zed agent runner.",
		Long:    "Start the Helix Zed agent runner that processes Zed editor tasks. Replaces gptscript runner entirely.",
		Example: "helix zed-agent-runner",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := zedAgentRunner(cmd)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Info().Msg("Zed agent runner stopped")
					return nil
				}
				return err
			}
			return nil
		},
	}

	return runCmd
}

func zedAgentRunner(_ *cobra.Command) error {
	cfg, err := config.LoadExternalAgentRunnerConfig()
	if err != nil {
		log.Error().Err(err).Msg("failed to load Zed agent runner config")
		return err
	}

	log.Info().
		Str("api_host", cfg.APIHost).
		Str("runner_id", cfg.RunnerID).
		Int("concurrency", cfg.Concurrency).
		Int("max_tasks", cfg.MaxTasks).
		Msg("starting Zed agent runner")

	// TODO: Fix pubsub and zedagent interface issues
	// Initialize pubsub for RDP data routing
	// pubsubInstance, err := pubsub.NewNats(&cfg.PubSub)
	// if err != nil {
	// 	log.Error().Err(err).Msg("failed to initialize pubsub")
	// 	return err
	// }

	// Initialize Zed runner using the new pattern
	// runner, err := zedagent.InitializeZedRunner(pubsubInstance)
	// if err != nil {
	// 	log.Error().Err(err).Msg("failed to initialize Zed runner")
	// 	return err
	// }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Set up signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info().
			Str("signal", sig.String()).
			Msg("received shutdown signal, stopping Zed agent runner")
		cancel()
	}()

	// TODO: Fix runner interface
	// Run the Zed agent runner
	log.Info().Msg("🟢 Zed agent runner temporarily disabled - needs interface fixes")

	// Wait for shutdown signal
	<-ctx.Done()
	log.Info().Msg("Zed agent runner gracefully shut down")

	// err = runner.Run(ctx)
	// if err != nil {
	// 	if errors.Is(err, context.Canceled) {
	// 		// Graceful shutdown
	// 		log.Info().Msg("Zed agent runner gracefully shut down")

	// 		// Give any active sessions time to finish
	// 		time.Sleep(2 * time.Second)

	// 		return nil
	// 	} else {
	// 		log.Error().Err(err).Msg("Zed agent runner stopped with error")
	// 		return err
	// 	}
	// }

	log.Info().Msg("Zed agent runner stopped successfully")
	return nil
}
