package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/revdial"
)

var (
	socketPath  string
	socketDir   string
	dataDir     string
	logLevel    string
	apiURL      string
	runnerToken string
	sandboxID   string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "hydra",
		Short: "Hydra multi-Docker daemon",
		Long: `Hydra is a daemon that manages multiple isolated Docker daemon instances.
Each external agent desktop gets its own dedicated dockerd for container isolation.

Docker data persists across lobby restarts and sandbox container restarts.
Running containers are NOT preserved across restarts - only images, volumes, and build cache.`,
		Run: run,
	}

	rootCmd.Flags().StringVar(&socketPath, "socket", hydra.DefaultSocketPath, "Path to Hydra Unix socket")
	rootCmd.Flags().StringVar(&socketDir, "socket-dir", hydra.DefaultSocketDir, "Directory for per-scope Docker sockets")
	rootCmd.Flags().StringVar(&dataDir, "data-dir", hydra.DefaultDataDir, "Directory for persistent Docker data")
	rootCmd.Flags().StringVar(&logLevel, "log-level", "info", "Log level (debug, info, warn, error)")

	// RevDial configuration (can also be set via environment variables)
	rootCmd.Flags().StringVar(&apiURL, "api-url", "", "Helix API URL for RevDial (env: HELIX_API_URL)")
	rootCmd.Flags().StringVar(&runnerToken, "token", "", "Runner authentication token (env: RUNNER_TOKEN)")
	rootCmd.Flags().StringVar(&sandboxID, "sandbox-id", "", "Sandbox instance ID (env: SANDBOX_INSTANCE_ID)")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal().Err(err).Msg("Failed to execute command")
	}
}

func run(cmd *cobra.Command, args []string) {
	// Configure logging
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}
	zerolog.SetGlobalLevel(level)

	// Use pretty logging for console output
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})

	log.Info().
		Str("socket", socketPath).
		Str("socket_dir", socketDir).
		Str("data_dir", dataDir).
		Str("log_level", logLevel).
		Bool("privileged_mode", os.Getenv("HYDRA_PRIVILEGED_MODE_ENABLED") == "true").
		Msg("Starting Hydra daemon")

	// Create manager and server
	manager := hydra.NewManager(socketDir, dataDir)
	server := hydra.NewServer(manager, socketPath)

	// Create context that cancels on SIGINT/SIGTERM
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigChan
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	// Start server
	if err := server.Start(ctx); err != nil {
		log.Fatal().Err(err).Msg("Failed to start Hydra server")
	}

	// Start RevDial client if configured
	// Environment variables take precedence over flags
	revDialAPIURL := apiURL
	if revDialAPIURL == "" {
		revDialAPIURL = os.Getenv("HELIX_API_URL")
	}
	revDialToken := runnerToken
	if revDialToken == "" {
		revDialToken = os.Getenv("RUNNER_TOKEN")
	}
	revDialSandboxID := sandboxID
	if revDialSandboxID == "" {
		revDialSandboxID = os.Getenv("SANDBOX_INSTANCE_ID")
	}
	if revDialSandboxID == "" {
		revDialSandboxID = "local"
	}

	var revDialClient *revdial.Client
	if revDialAPIURL != "" && revDialToken != "" {
		revDialClient = revdial.NewClient(&revdial.ClientConfig{
			ServerURL:          revDialAPIURL,
			RunnerID:           "hydra-" + revDialSandboxID,
			RunnerToken:        revDialToken,
			LocalAddr:          "unix://" + socketPath,
			ReconnectDelay:     5 * time.Second,
			InsecureSkipVerify: true, // TODO: make configurable
		})
		revDialClient.Start(ctx)
		log.Info().
			Str("runner_id", "hydra-"+revDialSandboxID).
			Msg("RevDial client started")
	} else {
		log.Info().Msg("RevDial not configured (no HELIX_API_URL or RUNNER_TOKEN)")
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	log.Info().Msg("Shutting down Hydra...")

	// Stop RevDial client
	if revDialClient != nil {
		revDialClient.Stop()
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer shutdownCancel()

	if err := server.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error during shutdown")
	}

	log.Info().Msg("Hydra daemon stopped")
}
