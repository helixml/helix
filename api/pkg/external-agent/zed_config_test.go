package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

// TestGenerateZedMCPConfig_AgentDefaultModel covers P1-1 from the Deviqon
// 2026-04-28 customer call. The original bug: when an agent had empty model
// fields, the API silently substituted anthropic/claude-sonnet-4-5-latest;
// when an agent referenced a renamed/deleted provider, the provider name
// was still encoded into the model string and Zed sent unroutable requests.
// Both paths caused customers to see "Claude Sonnet 4.5" in Zed when they
// thought they had configured Qwen on Scaleway.
//
// After the fix, GenerateZedMCPConfig leaves Agent.DefaultModel == nil for
// any misconfigured assistant. Zed falls back to its own built-in default
// rather than us pretending we configured one. Operators get a loud error
// log pointing at the broken agent.
func TestGenerateZedMCPConfig_AgentDefaultModel(t *testing.T) {
	ctx := context.Background()
	helixURL := "http://api:8080"
	helixToken := "test-token"

	cases := []struct {
		name             string
		assistants       []types.AssistantConfig // empty slice → no-assistant default-app path
		validProviders   []string
		wantDefaultModel *ModelConfig // nil = expect Agent.DefaultModel == nil
		wantMisconfig    bool         // expect ZedMCPConfig.Misconfigured to be set so handlers can return 422
		why              string
	}{
		{
			name:           "both_fields_empty_no_longer_falls_back_to_claude",
			assistants:     []types.AssistantConfig{{AgentType: types.AgentTypeZedExternal}},
			validProviders: []string{"openai", "anthropic"},
			// Sub-A fix: silent claude fallback removed. Empty fields => no default_model.
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-1 Sub-A: empty fields must not silently substitute Claude",
		},
		{
			name: "model_empty_provider_set_no_default_model",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "scaleway",
			}},
			validProviders:   []string{"scaleway", "openai"},
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-1 Sub-A: partial config (provider only) must not silently fill in claude-sonnet",
		},
		{
			name: "stale_provider_no_default_model",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "user-ollama",
				GenerationModel:         "qwen3-coder",
			}},
			// Provider was renamed/deleted in admin → not in registry anymore
			validProviders:   []string{"openai", "anthropic"},
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-1 Sub-B: stale provider snapshot must not be encoded into the model string",
		},
		{
			name: "configured_qwen_on_scaleway_works",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "scaleway",
				GenerationModel:         "qwen3-coder-480b",
			}},
			validProviders:   []string{"scaleway", "openai"},
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "scaleway/qwen3-coder-480b"},
			wantMisconfig:    false,
			why:              "control case: registered provider + non-empty model passes through unchanged",
		},
		{
			name: "configured_anthropic_passes_through",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-5",
			}},
			validProviders:   []string{"anthropic"},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5-latest"},
			wantMisconfig:    false,
			why:              "control case: anthropic agents normalize the model id via -latest",
		},
		{
			name: "case_insensitive_provider_match",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "OpenAI", // capital O as on prime row
				GenerationModel:         "gpt-5.4",
			}},
			validProviders:   []string{"openai"},
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "OpenAI/gpt-5.4"},
			wantMisconfig:    false,
			why:              "provider validation is case-insensitive (OpenAI vs openai)",
		},
		{
			name:             "no_assistant_keeps_legacy_default_for_default_app",
			assistants:       []types.AssistantConfig{},
			validProviders:   []string{"anthropic"},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5-latest"},
			wantMisconfig:    false,
			why:              "default-app path (no parent app) keeps the SaaS-friendly default",
		},
		{
			name: "nil_validProviders_skips_validation",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "scaleway",
				GenerationModel:         "qwen3-coder-480b",
			}},
			validProviders:   nil, // runner-side path passes nil
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "scaleway/qwen3-coder-480b"},
			wantMisconfig:    false,
			why:              "runner-side callers without a manager handle opt out of validation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &types.App{
				ID: "test-app",
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: tc.assistants,
					},
				},
			}

			cfg, err := GenerateZedMCPConfig(
				ctx,
				app,
				"user-1",
				"session-1",
				helixURL,
				helixToken,
				false,
				nil,
				nil,
				tc.validProviders,
			)
			assert.NoError(t, err)
			if !assert.NotNil(t, cfg) || !assert.NotNil(t, cfg.Agent) {
				return
			}
			if tc.wantDefaultModel == nil {
				assert.Nil(t, cfg.Agent.DefaultModel, tc.why)
				assert.Nil(t, cfg.Agent.InlineAssistantModel, tc.why)
				assert.Nil(t, cfg.Agent.CommitMessageModel, tc.why)
				assert.Nil(t, cfg.Agent.ThreadSummaryModel, tc.why)
			} else {
				if assert.NotNil(t, cfg.Agent.DefaultModel, tc.why) {
					assert.Equal(t, tc.wantDefaultModel.Provider, cfg.Agent.DefaultModel.Provider, tc.why)
					assert.Equal(t, tc.wantDefaultModel.Model, cfg.Agent.DefaultModel.Model, tc.why)
				}
			}
			assert.Equal(t, tc.wantMisconfig, cfg.Misconfigured, tc.why)
			if tc.wantMisconfig {
				assert.NotEmpty(t, cfg.MisconfigReason, "misconfigured config must include a human-readable reason for the 422 response")
			} else {
				assert.Empty(t, cfg.MisconfigReason, tc.why)
			}
		})
	}
}

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
