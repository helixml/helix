package external_agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNormalizeModelIDForZed(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Already normalized — should pass through unchanged
		{name: "already latest suffix", input: "claude-opus-4-6-latest", expected: "claude-opus-4-6-latest"},
		{name: "already latest suffix sonnet", input: "claude-sonnet-4-5-latest", expected: "claude-sonnet-4-5-latest"},

		// Actual Anthropic API model IDs (from /v1/models with anthropic-version header)
		{name: "claude-sonnet-4-6", input: "claude-sonnet-4-6", expected: "claude-sonnet-4-6-latest"},
		{name: "claude-opus-4-6", input: "claude-opus-4-6", expected: "claude-opus-4-6-latest"},
		{name: "claude-opus-4-5-20251101", input: "claude-opus-4-5-20251101", expected: "claude-opus-4-5-latest"},
		{name: "claude-haiku-4-5-20251001", input: "claude-haiku-4-5-20251001", expected: "claude-haiku-4-5-latest"},
		{name: "claude-sonnet-4-5-20250929", input: "claude-sonnet-4-5-20250929", expected: "claude-sonnet-4-5-latest"},
		{name: "claude-opus-4-1-20250805", input: "claude-opus-4-1-20250805", expected: "claude-opus-4-1-latest"},
		{name: "claude-opus-4-20250514", input: "claude-opus-4-20250514", expected: "claude-opus-4-latest"},
		{name: "claude-sonnet-4-20250514", input: "claude-sonnet-4-20250514", expected: "claude-sonnet-4-latest"},
		{name: "claude-3-haiku-20240307", input: "claude-3-haiku-20240307", expected: "claude-3-haiku-latest"},

		// Bare model names (no date suffix)
		{name: "bare claude-opus-4-1", input: "claude-opus-4-1", expected: "claude-opus-4-1-latest"},
		{name: "bare claude-opus-4", input: "claude-opus-4", expected: "claude-opus-4-latest"},
		{name: "bare claude-sonnet-4", input: "claude-sonnet-4", expected: "claude-sonnet-4-latest"},

		// 3.x models
		{name: "claude-3-5-sonnet date", input: "claude-3-5-sonnet-20241022", expected: "claude-3-5-sonnet-latest"},
		{name: "claude-3-5-haiku date", input: "claude-3-5-haiku-20241022", expected: "claude-3-5-haiku-latest"},
		{name: "claude-3-opus date", input: "claude-3-opus-20240229", expected: "claude-3-opus-latest"},
		{name: "claude-3-7-sonnet date", input: "claude-3-7-sonnet-20250219", expected: "claude-3-7-sonnet-latest"},

		// OpenAI models
		{name: "gpt-4o with date", input: "gpt-4o-2024-11-20", expected: "gpt-4o"},
		{name: "gpt-4o-mini with date", input: "gpt-4o-mini-2024-07-18", expected: "gpt-4o-mini"},
		{name: "gpt-4o bare", input: "gpt-4o", expected: "gpt-4o"},

		// Non-matching models pass through unchanged
		{name: "qwen model", input: "helix/qwen3:8b", expected: "helix/qwen3:8b"},
		{name: "gemini model", input: "gemini-2.0-flash", expected: "gemini-2.0-flash"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeModelIDForZed(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMergeZedConfigWithUserOverrides_AgentModelFieldsProtected verifies that
// the four helix-managed model fields under "agent" cannot be clobbered by
// user-side overrides (uploaded by the settings-sync-daemon when Zed writes
// to its local settings.json).
//
// See deviqon/P1-5-zed-overrides-clobber-helix-default-model.md.
func TestMergeZedConfigWithUserOverrides_AgentModelFieldsProtected(t *testing.T) {
	helixAgent := &AgentConfig{
		DefaultModel:           &ModelConfig{Provider: "openai", Model: "numpty/openai/gpt-oss-120b"},
		InlineAssistantModel:   &ModelConfig{Provider: "openai", Model: "numpty/openai/gpt-oss-120b"},
		CommitMessageModel:     &ModelConfig{Provider: "openai", Model: "numpty/openai/gpt-oss-120b"},
		ThreadSummaryModel:     &ModelConfig{Provider: "openai", Model: "numpty/openai/gpt-oss-120b"},
		AlwaysAllowToolActions: true,
		ShowOnboarding:         false,
		AutoOpenPanel:          true,
	}

	helixConfig := &ZedMCPConfig{
		ContextServers: map[string]ContextServerConfig{},
		Agent:          helixAgent,
	}

	t.Run("user override of default_model is dropped", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model": map[string]interface{}{
					"provider":        "anthropic",
					"model":           "claude-sonnet-4-6-latest",
					"effort":          "high",
					"enable_thinking": true,
				},
			},
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)

		agent, ok := merged["agent"].(map[string]interface{})
		assert.True(t, ok, "agent block should be a map")

		dm, ok := agent["default_model"].(*ModelConfig)
		assert.True(t, ok, "default_model should remain helix-managed *ModelConfig")
		assert.Equal(t, "openai", dm.Provider)
		assert.Equal(t, "numpty/openai/gpt-oss-120b", dm.Model)
	})

	t.Run("all four model fields are protected", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model":          map[string]interface{}{"provider": "anthropic", "model": "claude"},
				"inline_assistant_model": map[string]interface{}{"provider": "anthropic", "model": "claude"},
				"commit_message_model":   map[string]interface{}{"provider": "anthropic", "model": "claude"},
				"thread_summary_model":   map[string]interface{}{"provider": "anthropic", "model": "claude"},
			},
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)
		agent := merged["agent"].(map[string]interface{})

		for _, field := range []string{"default_model", "inline_assistant_model", "commit_message_model", "thread_summary_model"} {
			dm, ok := agent[field].(*ModelConfig)
			assert.True(t, ok, "%s should remain helix-managed", field)
			assert.Equal(t, "openai", dm.Provider, "%s.provider", field)
			assert.Equal(t, "numpty/openai/gpt-oss-120b", dm.Model, "%s.model", field)
		}
	})

	t.Run("non-model agent fields can still be user-overridden", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"agent": map[string]interface{}{
				"default_model":  map[string]interface{}{"provider": "anthropic", "model": "claude"},
				"play_sound_when_agent_done": true,
				"button":                     false,
			},
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)
		agent := merged["agent"].(map[string]interface{})

		// helix-managed model is protected
		assert.Equal(t, "numpty/openai/gpt-oss-120b", agent["default_model"].(*ModelConfig).Model)
		// arbitrary user-side keys land in the merged agent block
		assert.Equal(t, true, agent["play_sound_when_agent_done"])
		assert.Equal(t, false, agent["button"])
	})

	t.Run("top-level non-agent overrides still apply", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"theme":  "Dracula",
			"keymap": "Vim",
			"agent": map[string]interface{}{
				"default_model": map[string]interface{}{"provider": "anthropic", "model": "claude"},
			},
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)

		assert.Equal(t, "Dracula", merged["theme"])
		assert.Equal(t, "Vim", merged["keymap"])
		// Helix model still wins
		assert.Equal(t, "numpty/openai/gpt-oss-120b",
			merged["agent"].(map[string]interface{})["default_model"].(*ModelConfig).Model)
	})

	t.Run("no agent override surfaces helix agent block verbatim", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"theme": "Light",
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)

		// When the user has no agent override, the helix *AgentConfig is exposed directly.
		agent, ok := merged["agent"].(*AgentConfig)
		assert.True(t, ok, "agent should be the helix *AgentConfig when user has no override")
		assert.Equal(t, "numpty/openai/gpt-oss-120b", agent.DefaultModel.Model)
	})

	t.Run("non-object agent override is ignored", func(t *testing.T) {
		userOverrides := map[string]interface{}{
			"agent": "not-an-object",
		}
		merged := MergeZedConfigWithUserOverrides(helixConfig, userOverrides)

		agent, ok := merged["agent"].(*AgentConfig)
		assert.True(t, ok, "agent should fall back to helix *AgentConfig when user override is not a map")
		assert.Equal(t, "numpty/openai/gpt-oss-120b", agent.DefaultModel.Model)
	})
}
