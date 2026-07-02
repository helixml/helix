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

	// Synthetic globals (no ID) and DB-backed providers (with ID) used by
	// the cases below. Renames are demonstrated by mutating the .Name of a
	// DB-backed provider while keeping its .ID stable; the agent's stored
	// reference (the .ID) survives the rename.
	var (
		globalOpenAI    = ProviderRef{ID: "", Name: "openai"}
		globalAnthropic = ProviderRef{ID: "", Name: "anthropic"}
		dbScalewayID    = "pe_scaleway_01"
		dbScaleway      = ProviderRef{ID: dbScalewayID, Name: "scaleway"}
		dbScalewayPrime = ProviderRef{ID: dbScalewayID, Name: "scaleway-prime"} // same ID, renamed
		dbGLMID         = "pe_glm_01"
		dbGLM           = ProviderRef{ID: dbGLMID, Name: "glm-helix"}
	)

	cases := []struct {
		name             string
		assistants       []types.AssistantConfig // empty slice → no-assistant default-app path
		snapshot         []ProviderRef
		wantDefaultModel *ModelConfig // nil = expect Agent.DefaultModel == nil
		wantMisconfig    bool         // expect ZedMCPConfig.Misconfigured to be set so handlers can return 422
		why              string
	}{
		{
			name:             "both_fields_empty_no_longer_falls_back_to_claude",
			assistants:       []types.AssistantConfig{{AgentType: types.AgentTypeZedExternal}},
			snapshot:         []ProviderRef{globalOpenAI, globalAnthropic},
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-1 Sub-A: empty fields must not silently substitute Claude",
		},
		{
			name: "model_empty_provider_set_no_default_model",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: dbScalewayID,
			}},
			snapshot:         []ProviderRef{dbScaleway, globalOpenAI},
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-1 Sub-A: partial config (provider only) must not silently fill in claude-sonnet",
		},
		{
			name: "deleted_provider_no_default_model",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "pe_user_ollama_01", // ID, but provider was deleted
				GenerationModel:         "qwen3-coder",
			}},
			snapshot:         []ProviderRef{globalOpenAI, globalAnthropic},
			wantDefaultModel: nil,
			wantMisconfig:    true,
			why:              "P1-3: deleted provider must not be encoded into the model string",
		},
		{
			name: "rename_is_no_op_id_still_resolves",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: dbScalewayID, // agent stored ID
				GenerationModel:         "qwen3-coder-480b",
			}},
			snapshot:         []ProviderRef{dbScalewayPrime, globalOpenAI}, // admin renamed scaleway → scaleway-prime
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "scaleway-prime/qwen3-coder-480b"},
			wantMisconfig:    false,
			why:              "P1-3 core: provider rename must be a no-op for the agent — ID resolves to current name",
		},
		{
			name: "configured_qwen_on_scaleway_works",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: dbScalewayID,
				GenerationModel:         "qwen3-coder-480b",
			}},
			snapshot:         []ProviderRef{dbScaleway, globalOpenAI},
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "scaleway/qwen3-coder-480b"},
			wantMisconfig:    false,
			why:              "control case: agent stored ID resolves to canonical scaleway name",
		},
		{
			// Regression (2026-07-02, meta.helix.ml): a GLM-on-Helix external
			// agent booted Zed as openai/gpt-4o. The real pick lived in
			// Model/Provider while the helix_agent template defaults
			// (gpt-4o/openai) sat in the GenerationModel quartet. The reader
			// preferred GenerationModel and shadowed the real selection.
			name: "model_provider_wins_over_stale_generation_quartet",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				Provider:                dbGLMID,
				Model:                   "glm-5.1",
				GenerationModelProvider: "openai", // stale template default
				GenerationModel:         "gpt-4o", // stale template default
			}},
			snapshot:         []ProviderRef{dbGLM, globalOpenAI},
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "glm-helix/glm-5.1"},
			wantMisconfig:    false,
			why:              "zed_external source of truth is Model/Provider; the GenerationModel quartet must not shadow it",
		},
		{
			name: "configured_anthropic_passes_through",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-5",
			}},
			snapshot:         []ProviderRef{globalAnthropic},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5-latest"},
			wantMisconfig:    false,
			why:              "control case: env-baked global (no ID) resolves by canonical name; -latest normalization applies",
		},
		{
			name: "legacy_name_match_still_works_for_unsaved_agents",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "OpenAI", // capital O — legacy agent stored a name
				GenerationModel:         "gpt-5.4",
			}},
			snapshot:         []ProviderRef{globalOpenAI}, // global has Name=openai
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "openai/gpt-5.4"},
			wantMisconfig:    false,
			why:              "legacy fallback: agents stored before ID-based references still resolve via case-insensitive name match",
		},
		{
			name:             "no_assistant_keeps_legacy_default_for_default_app",
			assistants:       []types.AssistantConfig{},
			snapshot:         []ProviderRef{globalAnthropic},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5-latest"},
			wantMisconfig:    false,
			why:              "default-app path (no parent app) keeps the SaaS-friendly default",
		},
		{
			name: "nil_snapshot_skips_resolution",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "scaleway", // runner-side: name passed verbatim
				GenerationModel:         "qwen3-coder-480b",
			}},
			snapshot:         nil, // runner-side path passes nil
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "scaleway/qwen3-coder-480b"},
			wantMisconfig:    false,
			why:              "runner-side callers without a manager handle opt out of resolution and pass through verbatim",
		},
		{
			name: "subscription_agent_no_default_model_written",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			}},
			snapshot:         []ProviderRef{globalAnthropic},
			wantDefaultModel: nil,
			wantMisconfig:    false,
			why:              "subscription agents auth upstream directly; Zed must use its built-in defaults rather than a Helix-routed model. wantMisconfig=false so the spec-task entry handler does not 422.",
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
				tc.snapshot,
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
		{name: "claude-opus-4-8", input: "claude-opus-4-8", expected: "claude-opus-4-8-latest"},
		{name: "claude-opus-4-7", input: "claude-opus-4-7", expected: "claude-opus-4-7-latest"},
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

// TestMigrateLegacyProviderRefs covers the on-the-fly heal path that lets
// agent records saved before the ID-based refactor silently rewrite their
// stored provider name to the matching DB-backed provider's immutable ID.
// Renames after the heal are no-ops; without the heal the first rename
// would 422 the agent.
func TestMigrateLegacyProviderRefs(t *testing.T) {
	pe := ProviderRef{ID: "pe_user_provider_01", Name: "NVIDIA NIM"}
	openai := ProviderRef{ID: "", Name: "openai"} // env-baked global

	cases := []struct {
		name           string
		assistant      types.AssistantConfig
		snapshot       []ProviderRef
		wantChanged    bool
		wantProvider   string
		wantGenericGen string
		why            string
	}{
		{
			name: "legacy_name_to_id_rewrite",
			assistant: types.AssistantConfig{
				Provider: "NVIDIA NIM",
				Model:    "openai/gpt-oss-120b",
			},
			snapshot:     []ProviderRef{pe, openai},
			wantChanged:  true,
			wantProvider: "pe_user_provider_01",
			why:          "legacy stored name resolves to DB-backed ID and gets rewritten",
		},
		{
			name: "id_already_present_no_op",
			assistant: types.AssistantConfig{
				Provider: "pe_user_provider_01",
				Model:    "openai/gpt-oss-120b",
			},
			snapshot:     []ProviderRef{pe, openai},
			wantChanged:  false,
			wantProvider: "pe_user_provider_01",
			why:          "ID already stored — resolver returns byLegacy=false, no rewrite",
		},
		{
			name: "global_no_rewrite",
			assistant: types.AssistantConfig{
				Provider: "openai",
				Model:    "gpt-4o",
			},
			snapshot:     []ProviderRef{pe, openai},
			wantChanged:  false,
			wantProvider: "openai",
			why:          "env-baked global has no ID — leave the canonical name alone",
		},
		{
			name: "deleted_provider_left_alone",
			assistant: types.AssistantConfig{
				Provider: "pe_deleted_01",
				Model:    "qwen3-coder",
			},
			snapshot:     []ProviderRef{pe, openai},
			wantChanged:  false,
			wantProvider: "pe_deleted_01",
			why:          "resolver miss — no ID to write, leave the field for the validator to flag",
		},
		{
			name: "case_insensitive_legacy_match_rewrites",
			assistant: types.AssistantConfig{
				Provider: "nvidia nim", // lowercased legacy save
				Model:    "openai/gpt-oss-120b",
			},
			snapshot:     []ProviderRef{pe, openai},
			wantChanged:  true,
			wantProvider: "pe_user_provider_01",
			why:          "case-insensitive name match still triggers the rewrite to canonical ID",
		},
		{
			name: "generation_field_also_rewrites",
			assistant: types.AssistantConfig{
				GenerationModelProvider: "NVIDIA NIM",
				GenerationModel:         "openai/gpt-oss-120b",
			},
			snapshot:       []ProviderRef{pe, openai},
			wantChanged:    true,
			wantGenericGen: "pe_user_provider_01",
			why:            "GenerationModelProvider migrates the same way as the legacy Provider field",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &types.App{
				ID: "test-app",
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{tc.assistant},
					},
				},
			}
			changed := MigrateLegacyProviderRefs(app, tc.snapshot)
			assert.Equal(t, tc.wantChanged, changed, tc.why)
			if tc.wantProvider != "" {
				assert.Equal(t, tc.wantProvider, app.Config.Helix.Assistants[0].Provider, tc.why)
			}
			if tc.wantGenericGen != "" {
				assert.Equal(t, tc.wantGenericGen, app.Config.Helix.Assistants[0].GenerationModelProvider, tc.why)
			}
		})
	}
}

func TestMergeContextServers(t *testing.T) {
	helix := map[string]ContextServerConfig{
		"helix-desktop": {URL: "http://api:8080/api/v1/mcp/desktop", Headers: map[string]string{"Authorization": "Bearer x"}},
		"chrome":        {Command: "npx", Args: []string{"chrome-devtools-mcp@latest"}},
	}

	t.Run("user-only servers are added", func(t *testing.T) {
		got := MergeContextServers(helix, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"my-tool": map[string]interface{}{"command": "/usr/bin/mytool"},
			},
		})
		assert.Contains(t, got, "helix-desktop")
		assert.Contains(t, got, "chrome")
		assert.Contains(t, got, "my-tool")
	})

	t.Run("user override of helix server replaces it", func(t *testing.T) {
		got := MergeContextServers(helix, map[string]interface{}{
			"context_servers": map[string]interface{}{
				"chrome": map[string]interface{}{"command": "/custom/chrome"},
			},
		})
		assert.Equal(t, "/custom/chrome", got["chrome"].(map[string]interface{})["command"])
	})

	t.Run("no user overrides preserves helix servers", func(t *testing.T) {
		got := MergeContextServers(helix, map[string]interface{}{})
		assert.Contains(t, got, "helix-desktop")
		assert.Contains(t, got, "chrome")
	})

	t.Run("non-context_servers user keys are ignored", func(t *testing.T) {
		got := MergeContextServers(helix, map[string]interface{}{
			"agent":          map[string]interface{}{"default_model": "claude"},
			"language_models": map[string]interface{}{},
		})
		assert.NotContains(t, got, "agent")
		assert.NotContains(t, got, "language_models")
	})
}

func TestBuildLanguageModels(t *testing.T) {
	const helixURL = "http://api:8080"

	cases := []struct {
		name     string
		snapshot []ProviderRef
		want     map[string]LanguageModelConfig
	}{
		{
			name:     "nil snapshot preserves legacy both-providers behaviour",
			snapshot: nil,
			want: map[string]LanguageModelConfig{
				"anthropic": {APIURL: helixURL},
				"openai":    {APIURL: helixURL + "/v1"},
			},
		},
		{
			name:     "empty snapshot injects nothing",
			snapshot: []ProviderRef{},
			want:     map[string]LanguageModelConfig{},
		},
		{
			name:     "openai-only does not inject anthropic",
			snapshot: []ProviderRef{{Name: "openai"}},
			want: map[string]LanguageModelConfig{
				"openai": {APIURL: helixURL + "/v1"},
			},
		},
		{
			name:     "anthropic-only does not inject openai",
			snapshot: []ProviderRef{{Name: "anthropic"}},
			want: map[string]LanguageModelConfig{
				"anthropic": {APIURL: helixURL},
			},
		},
		{
			name: "both global providers inject both entries",
			snapshot: []ProviderRef{
				{Name: "openai"},
				{Name: "anthropic"},
			},
			want: map[string]LanguageModelConfig{
				"anthropic": {APIURL: helixURL},
				"openai":    {APIURL: helixURL + "/v1"},
			},
		},
		{
			name: "non-anthropic custom provider unlocks openai entry only",
			snapshot: []ProviderRef{
				{ID: "p_nebius", Name: "Nebius EU"},
			},
			want: map[string]LanguageModelConfig{
				"openai": {APIURL: helixURL + "/v1"},
			},
		},
		{
			name: "case insensitive anthropic match",
			snapshot: []ProviderRef{
				{Name: "Anthropic"},
				{Name: "OpenAI"},
			},
			want: map[string]LanguageModelConfig{
				"anthropic": {APIURL: helixURL},
				"openai":    {APIURL: helixURL + "/v1"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildLanguageModels(tc.snapshot, helixURL)
			assert.Equal(t, tc.want, got)
		})
	}
}

// TestValidateAssistantModelConfig_SubscriptionBypass guards the carve-out for
// subscription-credential agents (e.g. Claude Code with OAuth). These agents
// deliberately ship empty provider/model — the upstream auth lives in the
// container, not in a Helix provider — so the validator must not 422 them.
// The api_key cases stay as regression guards so we don't silently widen the
// bypass to runtimes that DO need a Helix-routed provider.
func TestValidateAssistantModelConfig_SubscriptionBypass(t *testing.T) {
	globalAnthropic := ProviderRef{ID: "", Name: "anthropic"}

	cases := []struct {
		name      string
		assistant types.AssistantConfig
		snapshot  []ProviderRef
		wantValid bool // true = no error returned (config considered valid)
		why       string
	}{
		{
			name: "subscription_empty_fields_ok",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			},
			snapshot:  []ProviderRef{globalAnthropic},
			wantValid: true,
			why:       "subscription agents auth upstream directly; empty provider/model is the documented shape",
		},
		{
			name: "subscription_populated_fields_also_ok",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
				GenerationModelProvider: "anthropic",
				GenerationModel:         "claude-sonnet-4-5",
			},
			snapshot:  []ProviderRef{globalAnthropic},
			wantValid: true,
			why:       "even with stored fields, subscription bypass short-circuits validation",
		},
		{
			name: "api_key_empty_fields_still_errors",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			},
			snapshot:  []ProviderRef{globalAnthropic},
			wantValid: false,
			why:       "regression guard: api_key runtimes must still surface missing provider/model",
		},
		{
			name: "empty_credential_type_treated_as_api_key",
			assistant: types.AssistantConfig{
				AgentType:        types.AgentTypeZedExternal,
				CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
			},
			snapshot:  []ProviderRef{globalAnthropic},
			wantValid: false,
			why:       "default (empty) credential type is api_key per the type docs; validator must still catch misconfig",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := &types.App{
				ID: "test-app",
				Config: types.AppConfig{
					Helix: types.AppHelixConfig{
						Assistants: []types.AssistantConfig{tc.assistant},
					},
				},
			}
			got := ValidateAssistantModelConfig(app, tc.snapshot)
			if tc.wantValid {
				assert.Empty(t, got, tc.why)
			} else {
				assert.NotEmpty(t, got, tc.why)
			}
		})
	}
}
