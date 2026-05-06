package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/helix/helixclient"
)

// runBootstrap dispatches `helix-org bootstrap <target>`.
func runBootstrap(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: helix-org bootstrap <target>\n\nTargets:\n  helix-runtime  Verify Helix connectivity")
	}
	switch args[0] {
	case "helix-runtime":
		return runBootstrapHelixRuntime(args[1:])
	case "help", "-h", "--help":
		fmt.Fprintln(os.Stderr, "usage: helix-org bootstrap helix-runtime [--db <path>]")
		return nil
	default:
		return fmt.Errorf("unknown bootstrap target %q", args[0])
	}
}

// runBootstrapHelixRuntime is now minimal under the per-Worker-project
// model: there is no shared "helix-org" project to provision. Each AI
// Worker hire creates its own Helix project at activation time. All
// this command does today is verify that `helix.url` + `helix.api_key`
// resolve to a real Helix user — a fast pre-flight before any Worker
// activation tries to reach it.
//
// Future expansion: an explicit "create the owner Worker's project"
// step lands here once the per-Worker-project work in tools/spawner
// is in place.
func runBootstrapHelixRuntime(args []string) error {
	fs := flag.NewFlagSet("bootstrap helix-runtime", flag.ContinueOnError)
	dbPath := fs.String("db", "helix-org.db", "SQLite DB path.")
	if err := fs.Parse(args); err != nil {
		return err
	}

	r, _, err := openRegistry(*dbPath)
	if err != nil {
		return err
	}
	ctx := context.Background()

	baseURL, err := r.GetString(ctx, "helix.url")
	if err != nil {
		return fmt.Errorf("helix.url not set (run `helix-org config set helix.url ...`): %w", err)
	}
	apiKey, err := r.GetString(ctx, "helix.api_key")
	if err != nil {
		return fmt.Errorf("helix.api_key not set: %w", err)
	}

	c, err := helixclient.New(helixclient.Config{BaseURL: baseURL, APIKey: apiKey})
	if err != nil {
		return fmt.Errorf("helix client: %w", err)
	}

	logf("→ pinging %s", baseURL)
	user, err := c.WhoAmI(ctx)
	if err != nil {
		return fmt.Errorf("helix unreachable: %w", err)
	}
	logf("  ok (user=%s slug=%s admin=%v)", user.User, user.Slug, user.Admin)
	logf("✓ bootstrap complete")
	logf("")
	logf("note: the per-Worker-project model means each AI Worker hire creates its own")
	logf("      Helix project at activation time. No global project to provision.")
	return nil
}

// persistString writes a single string value to the config registry.
// Kept here for future expansion (owner-project ID persistence, etc.).
func persistString(ctx context.Context, r *config.Registry, key, value string) error {
	encoded, _ := json.Marshal(value)
	if err := r.Set(ctx, key, string(encoded), ""); err != nil {
		return fmt.Errorf("persist %s: %w", key, err)
	}
	logf("  → set %s = %s", key, value)
	return nil
}

func logf(format string, args ...any) {
	_, _ = fmt.Fprintf(os.Stdout, format+"\n", args...)
}

// sandboxStartupSh is reserved for future use under the per-Worker
// model — when the spawner's TriggerHire step writes
// `.helix/startup.sh` per project, this is the canonical content.
const sandboxStartupSh = `#!/usr/bin/env bash
set -euo pipefail

# HELIX_ORG_URL / HELIX_WORKER_ID arrive as project secrets and surface
# as env vars in this container at session start. Use them to wire the
# in-sandbox claude/zed agent at the helix-org MCP endpoint.

mkdir -p ~/.config/claude
cat > ~/.config/claude/mcp.json <<EOF
{
  "mcpServers": {
    "helix": {
      "type": "http",
      "url": "${HELIX_ORG_URL}/workers/${HELIX_WORKER_ID}/mcp"
    }
  }
}
EOF
`

// sandboxStartupSh is reserved for the future per-Worker project
// apply step that drops it onto each new Helix project's helix-specs
// branch.
var _ = sandboxStartupSh
var _ = persistString
