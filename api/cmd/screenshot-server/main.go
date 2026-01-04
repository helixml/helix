package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/helixml/helix/api/pkg/desktop"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))

	cfg := desktop.Config{
		HTTPPort:       os.Getenv("SCREENSHOT_PORT"),
		WolfSocketPath: os.Getenv("WOLF_SOCKET_PATH"),
		XDGRuntimeDir:  os.Getenv("XDG_RUNTIME_DIR"),
		SessionID:      os.Getenv("HELIX_SESSION_ID"),
	}

	// Apply defaults
	if cfg.HTTPPort == "" {
		cfg.HTTPPort = "9876"
	}
	if cfg.WolfSocketPath == "" {
		cfg.WolfSocketPath = "/var/run/wolf/lobby.sock"
	}
	if cfg.XDGRuntimeDir == "" {
		cfg.XDGRuntimeDir = "/tmp/sockets"
	}

	// MCP server config
	mcpPort := os.Getenv("MCP_PORT")
	if mcpPort == "" {
		mcpPort = "9878" // Use 9878 to avoid conflict with settings-sync-daemon (9877)
	}
	mcpEnabled := os.Getenv("MCP_ENABLED") != "false" // Enabled by default

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	// Start screenshot server
	server := desktop.NewServer(cfg, logger)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := server.Run(ctx); err != nil && err != context.Canceled {
			logger.Error("screenshot server error", "err", err)
		}
	}()

	// Start MCP server if enabled
	if mcpEnabled {
		mcpCfg := desktop.MCPConfig{
			Port:          mcpPort,
			ScreenshotURL: fmt.Sprintf("http://localhost:%s/screenshot", cfg.HTTPPort),
		}
		mcpServer := desktop.NewMCPServer(mcpCfg, logger)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("starting MCP desktop server", "port", mcpPort)
			if err := mcpServer.Run(ctx, mcpPort); err != nil && err != context.Canceled {
				logger.Error("MCP server error", "err", err)
			}
		}()
	}

	// Wait for all servers to finish
	wg.Wait()
}
