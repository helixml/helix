package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepSeekHarnessAgentServerConfig(t *testing.T) {
	d := &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{
			Runtime: "deepseek_harness",
			BaseURL: "http://outer-api:8080/v1",
			Model:   "deepseek/deepseek-v4-pro",
		},
		userAPIKey: "hl-test-key",
	}

	cfg := d.generateAgentServerConfig()
	dsh, ok := cfg["dsh"].(map[string]interface{})
	assert.True(t, ok, "agent_servers must contain a dsh entry for the deepseek_harness runtime")
	assert.Equal(t, "custom", dsh["type"])
	assert.Equal(t, DeepSeekHarnessCommand, dsh["command"],
		"Zed must launch the wrapper, not `dsh`: upstream's product CLI has no acp subcommand")
	assert.Equal(t, []string{}, dsh["args"],
		"the wrapper takes no arguments — the composition path is baked into it")

	env, ok := dsh["env"].(map[string]interface{})
	assert.True(t, ok, "dsh entry must carry an env map")
	assert.Equal(t, "http://outer-api:8080/v1", env["HELIX_BASE_URL"])
	assert.Equal(t, "deepseek/deepseek-v4-pro", env["HELIX_MODEL"],
		"the model keeps its provider prefix — that prefix is what the Helix proxy routes on")
	assert.Equal(t, "hl-test-key", env["HELIX_API_KEY"])
	assert.Equal(t, DeepSeekHarnessHome, env["DSH_HOME"])
	assert.Equal(t, DeepSeekHarnessSessionsDir, env["DSH_SESSIONS_ROOT"])
}

// Without a key pi-ai fails every request with MISSING_CREDENTIAL mid-turn.
// Emitting no agent_servers entry instead defers to the next poll, so a
// session that is still waiting for its credentials self-heals rather than
// launching an agent that cannot talk to a provider.
func TestDeepSeekHarnessDefersWithoutCredentials(t *testing.T) {
	for _, tc := range []struct {
		name    string
		baseURL string
		model   string
		apiKey  string
	}{
		{"no api key", "http://outer-api:8080/v1", "openai/gpt-5", ""},
		{"no base url", "", "openai/gpt-5", "hl-test-key"},
		{"no model", "http://outer-api:8080/v1", "", "hl-test-key"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := &SettingsDaemon{
				codeAgentConfig: &CodeAgentConfig{
					Runtime: "deepseek_harness",
					BaseURL: tc.baseURL,
					Model:   tc.model,
				},
				userAPIKey: tc.apiKey,
			}
			assert.Nil(t, d.generateAgentServerConfig(),
				"an unusable dsh config must produce no agent_servers entry at all")
		})
	}
}

// The control plane now emits the canonical sandbox proxy URL in the
// zed-config response, so the daemon uses CodeAgentConfig.BaseURL verbatim (no
// rewriting). The dsh harness must pass it through to HELIX_BASE_URL unchanged.
func TestDeepSeekHarnessUsesConfiguredBaseURL(t *testing.T) {
	d := &SettingsDaemon{
		apiURL: "http://helix-api.internal:18080",
		codeAgentConfig: &CodeAgentConfig{
			Runtime: "deepseek_harness",
			BaseURL: "http://helix-api.internal:18080/v1",
			Model:   "openai/gpt-5",
		},
		userAPIKey: "hl-test-key",
	}

	dsh, ok := d.generateAgentServerConfig()["dsh"].(map[string]interface{})
	assert.True(t, ok)
	env := dsh["env"].(map[string]interface{})
	assert.Equal(t, "http://helix-api.internal:18080/v1", env["HELIX_BASE_URL"])
}

// Zed forwards context_servers into ACP session/new, and dsh rejects a
// non-empty list with -32602, which fails session creation and hangs the task
// with nothing shown in the UI. Withholding them here is what keeps sessions
// creatable; every other runtime must still get the full set.
func TestContextServersWithheldOnlyFromDeepSeekHarness(t *testing.T) {
	servers := map[string]interface{}{
		"helix-session":   map[string]interface{}{"url": "http://api:8080/mcp/session"},
		"chrome-devtools": map[string]interface{}{"command": "/usr/bin/chrome-devtools-mcp"},
	}

	dsh := &SettingsDaemon{
		codeAgentConfig: &CodeAgentConfig{Runtime: "deepseek_harness"},
		contextServers:  servers,
	}
	assert.Empty(t, dsh.contextServersForZed(),
		"dsh sessions must reach Zed with no context_servers, or session/new fails with -32602")

	for _, runtime := range []string{"zed_agent", "qwen_code", "claude_code", "codex_cli", "goose_code", "opencode"} {
		other := &SettingsDaemon{
			codeAgentConfig: &CodeAgentConfig{Runtime: runtime},
			contextServers:  servers,
		}
		assert.Equal(t, servers, other.contextServersForZed(),
			"%s must keep its MCP servers", runtime)
	}
}

// The servers dsh loses from session/new are mounted in its own composition,
// so the capability is moved rather than dropped.
func TestDeepSeekHarnessMCPEntriesCoverBothTransports(t *testing.T) {
	entries := deepSeekHarnessMCPEntries(map[string]interface{}{
		"helix-session": map[string]interface{}{
			"url":     "http://api:8080/api/v1/mcp/session?session_id=ses_1",
			"headers": map[string]interface{}{"Authorization": "Bearer hl-secret"},
		},
		"chrome-devtools": map[string]interface{}{
			"command": "/usr/bin/chrome-devtools-mcp",
			"args":    []interface{}{"--viewport", "1280x800"},
			"env":     map[string]interface{}{"FOO": "bar"},
		},
		"broken": map[string]interface{}{"nonsense": true},
	})

	// Sorted, and the unusable server is skipped rather than half-configured.
	assert.Len(t, entries, 2)
	assert.Equal(t, "mcp-chrome-devtools", entries[0]["id"])
	assert.Equal(t, "mcp-helix-session", entries[1]["id"])

	stdio := entries[0]["config"].(map[string]interface{})
	assert.Equal(t, "stdio", stdio["transport"])
	assert.Equal(t, "chrome-devtools", stdio["serverName"])
	assert.Equal(t, "/usr/bin/chrome-devtools-mcp", stdio["command"])
	assert.Equal(t, []interface{}{"--viewport", "1280x800"}, stdio["args"])

	http := entries[1]["config"].(map[string]interface{})
	assert.Equal(t, "streamable-http", http["transport"])
	assert.Equal(t, "http://api:8080/api/v1/mcp/session?session_id=ses_1", http["url"])
	assert.Equal(t, map[string]interface{}{"Authorization": "Bearer hl-secret"}, http["headers"])
	assert.Equal(t, "@deepseek-ai/dsh-mcp-client", entries[1]["name"])
}

// A stale file would leak the previous agent's servers — and its bearer
// tokens — into this session, so an empty set must still overwrite.
func TestDeepSeekHarnessMCPConfigOverwritesStaleFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "helix-dsh", "mcp.cordis.json")

	assert.NoError(t, writeDeepSeekHarnessMCPConfig(path, map[string]interface{}{
		"helix-session": map[string]interface{}{"url": "http://api:8080/mcp/session"},
	}))
	first, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.Contains(t, string(first), "helix-session")

	assert.NoError(t, writeDeepSeekHarnessMCPConfig(path, nil))
	second, err := os.ReadFile(path)
	assert.NoError(t, err)
	assert.NotContains(t, string(second), "helix-session",
		"an agent switch must not leave the previous session's MCP servers mounted")
	assert.Equal(t, "[]", strings.TrimSpace(string(second)))

	// Read-only so cordis-plugin-include's write-back path stays off.
	info, err := os.Stat(path)
	assert.NoError(t, err)
	assert.Equal(t, os.FileMode(0o400), info.Mode().Perm())
}
