package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/helixml/helix-org/bootstrap"
	"github.com/helixml/helix-org/store/sqlite"
)

// runBootstrap creates the initial owner Worker by opening the SQLite
// store directly. It must be run before `helix-org serve` — the owner
// Worker is what every later MCP client dials into. Bootstrap is the
// one operation that cannot itself go through MCP, because there is no
// Worker to act as until it has run.
func runBootstrap(args []string) error {
	fs := flag.NewFlagSet("bootstrap", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite database path (use ':memory:' for ephemeral)")
	envsDir := fs.String("envs-dir", "./envs", "Directory under which each Worker's Environment lives.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	absEnvsDir, err := filepath.Abs(*envsDir)
	if err != nil {
		return fmt.Errorf("resolve envs-dir %q: %w", *envsDir, err)
	}
	if err := os.MkdirAll(absEnvsDir, 0o750); err != nil {
		return fmt.Errorf("create envs-dir %q: %w", absEnvsDir, err)
	}
	envPath := filepath.Join(absEnvsDir, "w-owner")
	if err := os.MkdirAll(envPath, 0o750); err != nil {
		return fmt.Errorf("create owner env %q: %w", envPath, err)
	}

	store, err := sqlite.Open(*dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}

	result, err := bootstrap.Run(context.Background(), store, bootstrap.Params{
		EnvironmentPath: envPath,
	})
	if err != nil {
		if errors.Is(err, bootstrap.ErrAlreadyInitialised) {
			return fmt.Errorf("bootstrap: %w (use a different --db or wipe the existing one)", err)
		}
		return fmt.Errorf("bootstrap: %w", err)
	}

	out, err := json.MarshalIndent(map[string]any{
		"workerId":        result.WorkerID,
		"roleId":          result.RoleID,
		"positionId":      result.PositionID,
		"environmentPath": result.EnvironmentPath,
	}, "", "  ")
	if err != nil {
		return fmt.Errorf("format result: %w", err)
	}
	if _, err := fmt.Fprintln(os.Stdout, string(out)); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
