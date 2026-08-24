package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
)

// Paths for the DeepSeek Harness (dsh) runtime. Vars, not consts, so tests can
// redirect them at a tempdir.
var (
	// DeepSeekHarnessCommand is the wrapper Dockerfile.ubuntu-helix installs.
	// It loads the ACP composition from /opt/helix/dsh. Upstream's product CLI
	// has no acp subcommand, so there is no "dsh acp" to call instead.
	DeepSeekHarnessCommand = "/usr/local/bin/dsh-acp"
	// DeepSeekHarnessHome is $DSH_HOME: skill discovery and the credential
	// store. Isolated under a helix- prefix so it cannot collide with a
	// harness a user installs themselves.
	DeepSeekHarnessHome = "/home/retro/.config/helix-dsh"
	// DeepSeekHarnessSessionsDir holds the JSONL session log and its derived
	// SQLite query index. Under the workspace so sessions survive a container
	// restart, matching QWEN_HOME and OpenCodeDataHome.
	DeepSeekHarnessSessionsDir = "/home/retro/work/.dsh-sessions"
	// DeepSeekHarnessMCPConfigPath is the loader-entry file the composition
	// includes. Kept in lockstep with the literal `path` in
	// desktop/shared/dsh/cordis.yml — the loader resolves an include's path
	// without evaluating js tags, so it cannot be passed through the
	// environment like the rest of the per-session values.
	DeepSeekHarnessMCPConfigPath = "/home/retro/.config/helix-dsh/mcp.cordis.json"
)

// contextServersForZed returns the MCP servers Zed may configure for this
// session.
//
// Every runtime but one gets the full set. DeepSeek Harness gets none: Zed
// forwards the project's context servers in `session/new.mcpServers`
// (mcp_servers_for_project in zed's agent_servers/src/acp.rs), and dsh's ACP
// transport rejects a non-empty list with
// `-32602 mcpServers is not supported`. That kills session creation, and Zed
// surfaces nothing to the UI — the task simply hangs. Zed has no per-agent
// filter and stdio MCP has no ACP capability bit to gate on, so withholding
// the servers here is the only lever on the Helix side.
//
// The servers are not lost: writeDeepSeekHarnessMCPConfig mounts the same set
// inside the agent's own composition. The cost is that Zed's built-in agent
// panel in a dsh session also has no MCP tools.
func (d *SettingsDaemon) contextServersForZed() map[string]interface{} {
	if d.codeAgentConfig != nil && d.codeAgentConfig.Runtime == "deepseek_harness" {
		return map[string]interface{}{}
	}
	return d.contextServers
}

// writeDeepSeekHarnessMCPConfig renders Helix's MCP servers as dsh loader
// entries — one @deepseek-ai/dsh-mcp-client instance per server, which is how
// dsh expects MCP servers to be configured.
//
// They cannot travel the way they do for every other harness. Zed puts the
// project's context servers in `session/new.mcpServers`, and dsh's ACP
// transport rejects a non-empty list outright with
// `-32602 mcpServers is not supported`, failing session creation and leaving
// the task hanging with no error surfaced to the UI. So the daemon withholds
// context_servers from settings.json for this runtime (see
// contextServersForZed) and writes the same set here instead. The model sees
// the usual `mcp__<server>__<tool>` names either way.
//
// Always writes, even with no servers: the composition includes this path
// unconditionally, and a stale file from a previous agent would otherwise
// leak that agent's servers — and its bearer tokens — into this session.
func writeDeepSeekHarnessMCPConfig(path string, contextServers map[string]interface{}) error {
	entries := deepSeekHarnessMCPEntries(contextServers)

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal dsh mcp entries: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", filepath.Dir(path), err)
	}

	// Write via temp + rename so the agent can never observe a half-written
	// file, and 0400 so cordis-plugin-include treats it as read-only and
	// leaves its write-back path alone: the daemon is the only writer.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".mcp.cordis-*.json")
	if err != nil {
		return fmt.Errorf("create temp dsh mcp config: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write dsh mcp config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close dsh mcp config: %w", err)
	}
	if err := os.Chmod(tmp.Name(), 0o400); err != nil {
		return fmt.Errorf("chmod dsh mcp config: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("install dsh mcp config: %w", err)
	}
	return nil
}

// deepSeekHarnessMCPEntries converts Zed's context_servers map into cordis
// loader entries. Zed's two shapes map onto dsh's two transports: a `command`
// is stdio, a `url` is streamable-http. A server matching neither is skipped
// with no entry rather than a half-configured one.
func deepSeekHarnessMCPEntries(contextServers map[string]interface{}) []map[string]interface{} {
	// Sorted so the file is byte-stable across syncs; an unchanged file means
	// cordis's file watcher does not churn the MCP connections.
	names := make([]string, 0, len(contextServers))
	for name := range contextServers {
		names = append(names, name)
	}
	sort.Strings(names)

	entries := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		server, ok := contextServers[name].(map[string]interface{})
		if !ok {
			log.Printf("dsh: skipping MCP server %q: unexpected config shape %T", name, contextServers[name])
			continue
		}

		config := map[string]interface{}{
			"serverName": name,
			// A server that is down must not take the whole agent with it.
			// The default is already false; stating it makes the intent
			// explicit next to the reconnect behaviour we rely on.
			"failOnStartupError": false,
		}

		switch {
		case server["command"] != nil:
			config["transport"] = "stdio"
			config["command"] = server["command"]
			if args, ok := server["args"]; ok {
				config["args"] = args
			}
			if env, ok := server["env"]; ok {
				config["env"] = env
			}
		case server["url"] != nil:
			config["transport"] = "streamable-http"
			config["url"] = server["url"]
			if headers, ok := server["headers"]; ok {
				config["headers"] = headers
			}
		default:
			log.Printf("dsh: skipping MCP server %q: neither a command nor a url", name)
			continue
		}

		entries = append(entries, map[string]interface{}{
			"id":     "mcp-" + name,
			"name":   "@deepseek-ai/dsh-mcp-client",
			"config": config,
		})
	}
	return entries
}

// buildDeepSeekHarnessEnv renders the environment the ACP composition reads.
//
// Unlike opencode (one JSON blob) or goose (a config file on disk), dsh takes
// its whole configuration from desktop/shared/dsh/cordis.yml, which resolves
// these four variables at boot. Keeping the composition in the image and only
// the values here means a config change is a reviewable repo diff rather than
// a Go string builder.
//
// baseURL must already have been through rewriteLocalhostURL: the agent runs
// in a different network namespace from the API, so a localhost URL from the
// session config would resolve to the container itself.
func buildDeepSeekHarnessEnv(baseURL, model, apiKey string) (map[string]interface{}, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("no base URL configured")
	}
	if model == "" {
		return nil, fmt.Errorf("no model configured")
	}
	// pi-ai resolves apiKeyEnv per request and fails the request with
	// MISSING_CREDENTIAL when the reference resolves to nothing. Catching it
	// here turns a mid-turn provider error into a startup log line naming the
	// cause.
	if apiKey == "" {
		return nil, fmt.Errorf("no API key available yet")
	}
	return map[string]interface{}{
		"HELIX_BASE_URL":    baseURL,
		"HELIX_MODEL":       model,
		"HELIX_API_KEY":     apiKey,
		"DSH_HOME":          DeepSeekHarnessHome,
		"DSH_SESSIONS_ROOT": DeepSeekHarnessSessionsDir,
	}, nil
}
