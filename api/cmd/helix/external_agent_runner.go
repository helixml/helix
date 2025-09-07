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
	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "config_loading").
		Msg("🔧 EXTERNAL_AGENT_DEBUG: Loading external agent runner configuration")

	cfg, err := config.LoadExternalAgentRunnerConfig()
	if err != nil {
		log.Error().
			Str("EXTERNAL_AGENT_DEBUG", "config_load_error").
			Err(err).
			Msg("❌ EXTERNAL_AGENT_DEBUG: Failed to load external agent runner config")
		return err
	}

	log.Info().
		Str("EXTERNAL_AGENT_DEBUG", "config_loaded").
		Str("api_host", cfg.APIHost).
		Str("api_token", func() string {
			if cfg.APIToken != "" {
				return "***set***"
			}
			return "***empty***"
		}()).
		Str("runner_id", cfg.RunnerID).
		Int("concurrency", cfg.Concurrency).
		Int("max_tasks", cfg.MaxTasks).
		Int("rdp_start_port", cfg.RDPStartPort).
		Str("workspace_dir", cfg.WorkspaceDir).
		Str("zed_binary", cfg.ZedBinary).
		Int("display_num", cfg.DisplayNum).
		Int("session_timeout", cfg.SessionTimeout).
		Int("max_sessions", cfg.MaxSessions).
		Msg("✅ EXTERNAL_AGENT_DEBUG: External agent runner configuration loaded successfully")

	// Create external agent runner
	runner := external_agent.NewExternalAgentRunner(&cfg)

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
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "cleanup_goroutine_stop").
					Msg("🛑 EXTERNAL_AGENT_DEBUG: Session cleanup goroutine stopping")
				return
			case <-ticker.C:
				// ZedExecutor cleanup no longer available
				log.Debug().
					Str("EXTERNAL_AGENT_DEBUG", "session_cleanup_tick").
					Msg("🔄 EXTERNAL_AGENT_DEBUG: Session cleanup skipped - ZedExecutor not available")
			}
		}
	}()

	// Start the runner in a goroutine
	runnerDone := make(chan error, 1)
	go func() {
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "runner_goroutine_start").
			Msg("🚀 EXTERNAL_AGENT_DEBUG: Starting external agent runner in goroutine")
		runnerDone <- runner.Run(ctx)
	}()

	// Wait for either shutdown signal or runner completion
	select {
	case sig := <-sigChan:
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "shutdown_signal").
			Str("signal", sig.String()).
			Msg("🛑 EXTERNAL_AGENT_DEBUG: Received shutdown signal, stopping external agent runner")
		cancel()

		// Wait for runner to stop gracefully or timeout
		select {
		case err := <-runnerDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error().
					Str("EXTERNAL_AGENT_DEBUG", "runner_stop_error").
					Err(err).
					Msg("❌ EXTERNAL_AGENT_DEBUG: External agent runner stopped with error")
				return err
			}
			log.Info().
				Str("EXTERNAL_AGENT_DEBUG", "runner_stopped_gracefully").
				Msg("✅ EXTERNAL_AGENT_DEBUG: External agent runner stopped gracefully")
		case <-time.After(10 * time.Second):
			log.Warn().
				Str("EXTERNAL_AGENT_DEBUG", "runner_stop_timeout").
				Msg("⏰ EXTERNAL_AGENT_DEBUG: External agent runner didn't stop gracefully, forcing shutdown")
		}

		// Cleanup all active sessions (ZedExecutor no longer available)
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "session_cleanup_skip").
			Msg("🧹 EXTERNAL_AGENT_DEBUG: Skipping session cleanup - ZedExecutor not available")

		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "shutdown_complete").
			Msg("✅ EXTERNAL_AGENT_DEBUG: External agent runner stopped successfully")
		return nil

	case err := <-runnerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error().
				Str("EXTERNAL_AGENT_DEBUG", "runner_exit_error").
				Err(err).
				Msg("❌ EXTERNAL_AGENT_DEBUG: External agent runner exited with error")
			return err
		}
		log.Info().
			Str("EXTERNAL_AGENT_DEBUG", "runner_completed").
			Msg("✅ EXTERNAL_AGENT_DEBUG: External agent runner completed")
		return nil
	}
}
