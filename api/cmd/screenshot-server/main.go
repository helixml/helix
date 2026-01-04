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

	// Desktop MCP server config (screenshot, clipboard, input, window management)
	desktopMCPPort := os.Getenv("DESKTOP_MCP_PORT")
	if desktopMCPPort == "" {
		desktopMCPPort = "9877" // Desktop MCP on 9877, Session MCP on 9878
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

	// Start Desktop MCP server if enabled (port 9877)
	if mcpEnabled {
		mcpCfg := desktop.MCPConfig{
			Port:          desktopMCPPort,
			ScreenshotURL: fmt.Sprintf("http://localhost:%s/screenshot", cfg.HTTPPort),
		}
		mcpServer := desktop.NewMCPServer(mcpCfg, logger)

		wg.Add(1)
		go func() {
			defer wg.Done()
			logger.Info("starting Desktop MCP server", "port", desktopMCPPort)
			if err := mcpServer.Run(ctx, desktopMCPPort); err != nil && err != context.Canceled {
				logger.Error("Desktop MCP server error", "err", err)
			}
		}()
	}

	// Wait for all servers to finish
	wg.Wait()
}
