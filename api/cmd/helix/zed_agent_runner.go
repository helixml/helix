package helix

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/pubsub"
	"github.com/helixml/helix/api/pkg/zedagent"
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
	cfg, err := config.LoadZedAgentRunnerConfig()
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

	// Initialize pubsub for RDP data routing
	pubsubInstance, err := pubsub.NewNats(&cfg.PubSub)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize pubsub")
		return err
	}

	// Initialize Zed runner using the new pattern
	runner, err := zedagent.InitializeZedRunner(pubsubInstance)
	if err != nil {
		log.Error().Err(err).Msg("failed to initialize Zed runner")
		return err
	}

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

	// Run the Zed agent runner
	log.Info().Msg("🟢 Zed agent runner started and ready for tasks")

	err = runner.Run(ctx)
	if err != nil {
		select {
		case <-ctx.Done():
			// Graceful shutdown
			log.Info().Msg("Zed agent runner gracefully shut down")

			// Give any active sessions time to finish
			time.Sleep(2 * time.Second)

			return nil
		default:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error().Err(err).Msg("Zed agent runner stopped with error")
				return err
			}
		}
	case <-time.After(10 * time.Second):
		log.Warn().Msg("Zed agent runner didn't stop gracefully, forcing shutdown")
	}

	log.Info().Msg("Zed agent runner stopped successfully")
	return nil
}
