package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

	server := desktop.NewServer(cfg, logger)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := server.Run(ctx); err != nil && err != context.Canceled {
		logger.Error("server error", "err", err)
		os.Exit(1)
	}
}
