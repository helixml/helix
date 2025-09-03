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
	"github.com/helixml/helix/api/pkg/gptscript"
)

func NewZedAgentRunnerCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:     "zed-agent-runner",
		Short:   "Start the helix Zed agent runner.",
		Long:    "Start the helix Zed agent runner that executes Zed editor instances as external agents.",
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
		Int("rdp_start_port", cfg.RDPStartPort).
		Str("workspace_dir", cfg.WorkspaceDir).
		Msg("starting Zed agent runner")

	// Create Zed executor
	zedExecutor := gptscript.NewZedExecutor(
		cfg.DisplayNum,   // Display base number
		cfg.RDPStartPort, // RDP port base
		cfg.VNCPort,      // VNC port base
		cfg.WorkspaceDir, // Workspace directory
	)

	// Convert ZedAgentRunnerConfig to GPTScriptRunnerConfig for compatibility
	// TODO: Create a proper ZedAgentRunner struct instead of reusing GPTScriptRunner
	gptCfg := config.GPTScriptRunnerConfig{
		APIHost:     cfg.APIHost,
		APIToken:    cfg.APIToken,
		RunnerID:    cfg.RunnerID,
		Concurrency: cfg.Concurrency,
		MaxTasks:    cfg.MaxTasks,
	}

	runner := gptscript.NewZedAgentRunner(&gptCfg, zedExecutor)

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
			Msg("received shutdown signal, stopping Zed agent runner")
		cancel()

		// Wait for runner to stop gracefully or timeout
		select {
		case err := <-runnerDone:
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Error().Err(err).Msg("Zed agent runner stopped with error")
				return err
			}
		case <-time.After(10 * time.Second):
			log.Warn().Msg("Zed agent runner didn't stop gracefully, forcing shutdown")
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

		log.Info().Msg("Zed agent runner stopped successfully")
		return nil

	case err := <-runnerDone:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Error().Err(err).Msg("Zed agent runner exited with error")
			return err
		}
		log.Info().Msg("Zed agent runner completed")
		return nil
	}
}
