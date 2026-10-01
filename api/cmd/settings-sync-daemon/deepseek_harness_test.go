package main

import (
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
