package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
	installClaudeMCP := fs.Bool("install-claude-mcp", false, "After bootstrap, register the owner Worker's MCP endpoint with the local Claude CLI so future `claude` sessions can talk to it.")
	claudeMCPName := fs.String("claude-mcp-name", "helix-org", "Name to register the MCP server under in Claude's config (used with --install-claude-mcp).")
	claudeMCPScope := fs.String("claude-mcp-scope", "user", "Scope for the Claude MCP entry: local, project, or user (used with --install-claude-mcp).")
	claudeBin := fs.String("claude-bin", "claude", "Path to the claude CLI (used with --install-claude-mcp).")
	serverURL := fs.String("server-url", "http://localhost:8080", "Base URL the future `helix-org serve` will listen on; embedded into the Claude MCP entry.")
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

	if *installClaudeMCP {
		if err := installClaudeMCPEntry(*claudeBin, *claudeMCPName, *claudeMCPScope, *serverURL, string(result.WorkerID)); err != nil {
			return fmt.Errorf("install claude mcp: %w", err)
		}
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

// installClaudeMCPEntry registers the owner's MCP endpoint with the
// local Claude CLI by shelling out to `claude mcp add-json`. Once the
// entry is in place, plain `claude` sessions in that scope can call
// helix-org tools directly. If an entry with the same name already
// exists it is removed first so re-running bootstrap (e.g. between
// demo runs against different --db files) is idempotent.
func installClaudeMCPEntry(claudeBin, name, scope, serverURL, workerID string) error {
	mcpURL := fmt.Sprintf("%s/workers/%s/mcp", strings.TrimRight(serverURL, "/"), workerID)
	entry, err := json.Marshal(map[string]string{
		"type": "http",
		"url":  mcpURL,
	})
	if err != nil {
		return fmt.Errorf("marshal mcp entry: %w", err)
	}
	// Best-effort remove first — `claude mcp remove` exits non-zero
	// when the entry is missing, which is fine here.
	rm := exec.Command(claudeBin, "mcp", "remove", "--scope", scope, name) //nolint:gosec // operator-supplied flags
	_ = rm.Run()
	cmd := exec.Command(claudeBin, "mcp", "add-json", "--scope", scope, name, string(entry)) //nolint:gosec // claudeBin/scope/name are operator-supplied flags on a CLI tool the operator just invoked
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s mcp add-json: %w", claudeBin, err)
	}
	return nil
}
