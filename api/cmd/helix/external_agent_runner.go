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
	external_agent "github.com/helixml/helix/api/pkg/external-agent"
)

func NewExternalAgentRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "external-agent-runner",
		Short:   "Start the helix external agent runner.",
		Long:    "Start the helix external agent runner that executes Zed editor instances as external agents.",
		Example: "helix external-agent-runner",
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := externalAgentRunner(cmd)
			if err != nil {
				if errors.Is(err, context.Canceled) {
					log.Info().Msg("external agent runner stopped")
					return nil
				}
				return err
			}
			return nil
		},
	}

	return runCmd
}

func externalAgentRunner(_ *cobra.Command) error {
	cfg, err := config.LoadExternalAgentRunnerConfig()
	if err != nil {
		log.Error().Err(err).Msg("failed to load external agent runner config")
		return err
	}

	log.Info().
		Str("api_host", cfg.APIHost).
		Str("runner_id", cfg.RunnerID).
		Int("concurrency", cfg.Concurrency).
		Int("max_tasks", cfg.MaxTasks).
		Int("rdp_start_port", cfg.RDPStartPort).
		Str("workspace_dir", cfg.WorkspaceDir).
		Msg("starting external agent runner")

	// Create Zed executor
	zedExecutor := external_agent.NewZedExecutor(cfg.WorkspaceDir)

	runner := external_agent.NewExternalAgentRunner(&cfg, zedExecutor)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start session cleanup goroutine
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				timeout := time.Duration(cfg.SessionTimeout) * time.Second
				zedExecutor.CleanupExpiredSessions(ctx, timeout)
			}
		}
	}()

	// Start the runner in a goroutine
	runnerDone := make(chan error, 1)
	go func() {
		runnerDone <- runner.Run(ctx)
	}()

	// Wait for either shutdown signal or runner completion
	select {
	case sig := <-sigChan:
		log.Info().
			Str("signal", sig.String()).
			Msg("received shutdown signal, stopping external agent runner")
		cancel()

		// Wait for runner to stop gracefully or timeout
		select {
		case err := <-runnerDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error().Err(err).Msg("external agent runner stopped with error")
				return err
			}
		case <-time.After(10 * time.Second):
			log.Warn().Msg("external agent runner didn't stop gracefully, forcing shutdown")
		}

		// Cleanup all active sessions
		sessions := zedExecutor.ListSessions()
		for _, session := range sessions {
			if err := zedExecutor.StopZedAgent(context.Background(), session.SessionID); err != nil {
				log.Error().
					Err(err).
					Str("session_id", session.SessionID).
					Msg("failed to stop session during shutdown")
			}
		}

		log.Info().Msg("external agent runner stopped successfully")
		return nil

	case err := <-runnerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("external agent runner exited with error")
			return err
		}
		log.Info().Msg("external agent runner completed")
		return nil
	}
}
