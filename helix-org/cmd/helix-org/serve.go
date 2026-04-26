package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/helixml/helix-org/broadcast"
	"github.com/helixml/helix-org/dispatch"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
)

func runServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	addr := fs.String("addr", ":8080", "TCP address to listen on")
	dbPath := fs.String("db", "helix-org.db", "SQLite database path (use ':memory:' for ephemeral)")
	publicURL := fs.String("public-url", "", "Base URL spawned Workers use to reach the MCP endpoint. Defaults to http://localhost<addr-port>.")
	envsDir := fs.String("envs-dir", "./envs", "Directory under which each Worker's Environment lives (one subdirectory per workerID).")
	claudeBin := fs.String("claude-bin", "claude", "Path to the claude CLI used to embody AI Workers")
	model := fs.String("model", "", "Claude model to pass via --model (empty = let claude choose)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *publicURL == "" {
		*publicURL = "http://localhost" + portFromAddr(*addr)
	}
	absEnvsDir, err := filepath.Abs(*envsDir)
	if err != nil {
		return fmt.Errorf("resolve envs-dir %q: %w", *envsDir, err)
	}
	if err := os.MkdirAll(absEnvsDir, 0o750); err != nil {
		return fmt.Errorf("create envs-dir %q: %w", absEnvsDir, err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	store, err := sqlite.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	bc := broadcast.New()
	deps := tools.DefaultDeps(store)
	deps.Broadcaster = bc
	deps.EnvsDir = absEnvsDir

	spawner := tools.ClaudeSpawner(tools.ClaudeSpawnerConfig{
		ClaudeBin: *claudeBin,
		PublicURL: *publicURL,
		Model:     *model,
		Logger:    logger,
	})
	deps.Dispatcher = dispatch.New(store, spawner, logger)
	logger.Info("dispatcher enabled", "claude-bin", *claudeBin, "public-url", *publicURL, "envs-dir", absEnvsDir, "model", *model)

	reg := tools.NewRegistry()
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		return fmt.Errorf("register builtins: %w", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           server.New(store, reg, bc, logger).Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", *addr, "db", *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	case err, ok := <-errCh:
		if ok && err != nil {
			return fmt.Errorf("serve: %w", err)
		}
	}
	return nil
}

// portFromAddr extracts the ":PORT" suffix from a TCP address such as
// ":8080", "127.0.0.1:8080", or "0.0.0.0:8080". Returns ":8080" for an
// addr that has no explicit port (which mirrors net.http's own default).
func portFromAddr(addr string) string {
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		return addr[i:]
	}
	return ":8080"
}
