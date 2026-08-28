package external_agent

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
)

func TestGenerateZedMCPConfigAllowsUnsandboxedCommands(t *testing.T) {
	config, err := GenerateZedMCPConfig(
		context.Background(),
		&types.App{ID: "test-app"},
		"user-1",
		"session-1",
		"http://api:8080",
		"test-token",
		false,
		nil,
		nil,
		nil,
		"",
		nil,
	)
	assert.NoError(t, err)
	if assert.NotNil(t, config.Agent) {
		assert.True(t, config.Agent.AllowUnsandboxedCommands)
	}
}

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

	// Synthetic globals and DB-backed providers used by
	// the cases below. Renames are demonstrated by mutating the .Name of a
	// DB-backed provider while keeping its .ID stable; the agent's stored
	// reference (the .ID) survives the rename.
	var (
		globalOpenAI    = ProviderRef{ID: "global/openai", Name: "openai", EndpointType: types.ProviderEndpointTypeGlobal}
		globalAnthropic = ProviderRef{ID: "global/anthropic", Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal}
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
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "pe_scaleway_01/qwen3-coder-480b"},
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
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "pe_scaleway_01/qwen3-coder-480b"},
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
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "pe_glm_01/glm-5.1"},
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
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5"},
			wantMisconfig:    false,
			why:              "control case: env-baked global (no ID) resolves by canonical name; anthropic model id passes through verbatim to match Zed's /v1/models listing",
		},
		{
			name: "legacy_anthropic_task_ref_routes_to_org_endpoint",
			assistants: []types.AssistantConfig{{
				AgentType:        types.AgentTypeZedExternal,
				CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
				Provider:         "anthropic",
				Model:            "claude-sonnet-4-5",
			}},
			snapshot:         []ProviderRef{{ID: "global/anthropic", Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal}, {ID: "pe_org_anthropic", Name: "user/anthropic", EndpointType: types.ProviderEndpointTypeOrg}},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-5"},
			wantMisconfig:    false,
			why:              "legacy task refs must resolve to the organization Anthropic row and retain Anthropic routing",
		},
		{
			name: "explicit_global_openai_keeps_scoped_routing_token",
			assistants: []types.AssistantConfig{{
				AgentType: types.AgentTypeZedExternal,
				Provider:  "global/openai",
				Model:     "gpt-5.4",
			}},
			snapshot:         []ProviderRef{{ID: "pe_org_openai", Name: "user/openai", EndpointType: types.ProviderEndpointTypeOrg}, globalOpenAI},
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "global/openai/gpt-5.4"},
			wantMisconfig:    false,
			why:              "an explicit env-global selection must not degrade to a bare vendor token",
		},
		{
			name: "legacy_name_match_still_works_for_unsaved_agents",
			assistants: []types.AssistantConfig{{
				AgentType:               types.AgentTypeZedExternal,
				GenerationModelProvider: "OpenAI", // capital O — legacy agent stored a name
				GenerationModel:         "gpt-5.4",
			}},
			snapshot:         []ProviderRef{globalOpenAI}, // global has Name=openai
			wantDefaultModel: &ModelConfig{Provider: "openai", Model: "global/openai/gpt-5.4"},
			wantMisconfig:    false,
			why:              "legacy fallback: agents stored before ID-based references still resolve via case-insensitive name match",
		},
		{
			name:             "no_assistant_keeps_legacy_default_for_default_app",
			assistants:       []types.AssistantConfig{},
			snapshot:         []ProviderRef{globalAnthropic},
			wantDefaultModel: &ModelConfig{Provider: "anthropic", Model: "claude-sonnet-4-6"},
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
				"",
				nil,
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

func TestGenerateZedMCPConfigAddsDirectHelixOrgMCP(t *testing.T) {
	config, err := GenerateZedMCPConfig(
		context.Background(),
		&types.App{ID: "test-app", Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal,
			MCPs:      []types.AssistantMCP{{Name: "helix", URL: "http://old.example/mcp"}},
		}}}}},
		"user-1",
		"session-1",
		"http://sandbox-api:8080/",
		"session-token",
		false,
		nil,
		nil,
		nil,
		"b-worker",
		nil,
	)
	assert.NoError(t, err)
	assert.Equal(t, ContextServerConfig{
		URL: "http://sandbox-api:8080/api/v1/mcp/helix-org",
		Headers: map[string]string{
			"Authorization": "Bearer session-token",
		},
	}, config.ContextServers["helix"])
}

// TestMapHelixToZedProvider guards the model id Helix writes into
// agent.default_model. Zed resolves default_model by exact id against the
// provider's /v1/models listing; the stored id already comes from that listing,
// so anthropic models must pass through verbatim. Rewriting to a "-latest" alias
// (the old behaviour) matches nothing in the listing and makes Zed silently fall
// back to gpt-5-mini, which has no Helix route → empty agent responses.
func TestMapHelixToZedProvider(t *testing.T) {
	tests := []struct {
		name         string
		provider     string
		model        string
		wantProvider string
		wantModel    string
	}{
		// Anthropic — verbatim, NO -latest rewrite.
		{name: "opus-4-8 verbatim", provider: "anthropic", model: "claude-opus-4-8", wantProvider: "anthropic", wantModel: "claude-opus-4-8"},
		{name: "dated opus-4-5 verbatim", provider: "anthropic", model: "claude-opus-4-5-20251101", wantProvider: "anthropic", wantModel: "claude-opus-4-5-20251101"},
		{name: "provider case-insensitive", provider: "Anthropic", model: "claude-sonnet-4-6", wantProvider: "anthropic", wantModel: "claude-sonnet-4-6"},

		// OpenAI-compatible providers — prefixed so Helix routes to the backend.
		{name: "openai prefixed", provider: "openai", model: "gpt-4o", wantProvider: "openai", wantModel: "openai/gpt-4o"},
		{name: "nebius prefixed", provider: "nebius", model: "Qwen/Qwen3-Coder", wantProvider: "openai", wantModel: "nebius/Qwen/Qwen3-Coder"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotProvider, gotModel := mapHelixToZedProvider(tt.provider, tt.model)
			assert.Equal(t, tt.wantProvider, gotProvider)
			assert.Equal(t, tt.wantModel, gotModel)
		})
	}
}

func TestResolveProviderScopesLegacyAndExactRefs(t *testing.T) {
	snapshot := []ProviderRef{
		{ID: "global/anthropic", Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal},
		{ID: "pe_global", Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal},
		{ID: "pe_org", Name: "user/anthropic", EndpointType: types.ProviderEndpointTypeOrg},
	}

	legacy, _, ok := ResolveProvider("anthropic", snapshot)
	assert.True(t, ok)
	assert.Equal(t, "pe_org", legacy.ID)
	forcedGlobal, _, ok := ResolveProvider("global/anthropic", snapshot)
	assert.True(t, ok)
	assert.Equal(t, "global/anthropic", forcedGlobal.ID)
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
		{
			name: "legacy_anthropic_preset_name_rewrites_to_org_id",
			assistant: types.AssistantConfig{
				Provider: "anthropic",
				Model:    "claude-sonnet-4-5",
			},
			snapshot:     []ProviderRef{{ID: "pe_org_anthropic", Name: "user/anthropic"}},
			wantChanged:  true,
			wantProvider: "pe_org_anthropic",
			why:          "legacy Anthropic task refs heal to the visible organization endpoint ID",
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
		assert.Equal(t, "http://api:8080/api/v1/mcp/desktop", got["helix-desktop"].(map[string]interface{})["url"])
		assert.Equal(t, map[string]string{"Authorization": "Bearer x"}, got["helix-desktop"].(map[string]interface{})["headers"])
		assert.NotContains(t, got["helix-desktop"].(map[string]interface{}), "command")
		assert.Contains(t, got, "chrome")
	})

	t.Run("non-context_servers user keys are ignored", func(t *testing.T) {
		got := MergeContextServers(helix, map[string]interface{}{
			"agent":           map[string]interface{}{"default_model": "claude"},
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
			name:     "legacy anthropic preset name injects anthropic",
			snapshot: []ProviderRef{{ID: "pe_org_anthropic", Name: "user/anthropic"}},
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
			name: "codex_subscription_empty_fields_ok",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
			},
			snapshot:  []ProviderRef{globalAnthropic},
			wantValid: true,
			why:       "Codex CLI authenticates upstream with restored ChatGPT credentials",
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

func TestValidateAssistantModelConfig_ProviderAvailability(t *testing.T) {
	app := func(orgID string) *types.App {
		return &types.App{
			ID:             "app",
			OrganizationID: orgID,
			Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
				AgentType: types.AgentTypeZedExternal,
				Provider:  "pe_provider",
				Model:     "model",
			}}}},
		}
	}

	assert.Equal(t, types.OrganizationProviderUnavailableMessage, ValidateAssistantModelConfig(app("org_id"), []ProviderRef{}))
	assert.Contains(t, ValidateAssistantModelConfig(app(""), []ProviderRef{}), "does not match any current provider")
	assert.Empty(t, ValidateAssistantModelConfig(app("org_id"), []ProviderRef{{ID: "pe_provider", Name: "provider"}}))

	claudeCodeAPIKeyApp := &types.App{
		ID:             "app",
		OrganizationID: "org_id",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider:                "anthropic",
			Model:                   "claude-opus-4-8",
			GenerationModelProvider: "pe_personal",
			GenerationModel:         "scope-e2e-model",
		}}}},
	}
	assert.Equal(t, types.OrganizationProviderUnavailableMessage, ValidateAssistantModelConfig(claudeCodeAPIKeyApp, []ProviderRef{{Name: "anthropic"}}))
	assert.Contains(t,
		ValidateAssistantModelConfig(claudeCodeAPIKeyApp, []ProviderRef{{Name: "anthropic"}, {ID: "pe_personal", Name: "scope-good"}}),
		"requires the anthropic provider")
	claudeCodeAPIKeyApp.Config.Helix.Assistants[0].GenerationModelProvider = "anthropic"
	assert.Empty(t, ValidateAssistantModelConfig(claudeCodeAPIKeyApp, []ProviderRef{{Name: "anthropic"}}))

	codexAPIKeyApp := &types.App{
		ID: "codex-app",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider:                "pe_provider",
			Model:                   "gpt-5.6-sol",
		}}}},
	}
	assert.Contains(t,
		ValidateAssistantModelConfig(codexAPIKeyApp, []ProviderRef{{ID: "pe_provider", Name: "custom"}}),
		"requires the openai provider")
	assert.Empty(t,
		ValidateAssistantModelConfig(codexAPIKeyApp, []ProviderRef{{ID: "pe_provider", Name: "openai"}}))
}

func TestGenerateZedMCPConfigAddsSpecTaskMCP(t *testing.T) {
	tools := []string{"create_spectask", "list_spectasks"}
	config, err := GenerateZedMCPConfig(
		context.Background(),
		&types.App{ID: "test-app"},
		"user-1",
		"session-1",
		"http://sandbox-api:8080/",
		"session-token",
		false,
		nil,
		nil,
		nil,
		"",
		tools,
	)
	assert.NoError(t, err)
	assert.Equal(t, ContextServerConfig{
		URL:     "http://sandbox-api:8080/api/v1/mcp/helix-tasks?rev=" + AgentToolsRev(tools),
		Headers: map[string]string{"Authorization": "Bearer session-token"},
	}, config.ContextServers["helix-tasks"])
}

func TestGenerateZedMCPConfigOmitsSpecTaskMCPWithoutTools(t *testing.T) {
	config, err := GenerateZedMCPConfig(
		context.Background(),
		&types.App{ID: "test-app"},
		"user-1",
		"session-1",
		"http://sandbox-api:8080/",
		"session-token",
		false,
		nil,
		nil,
		nil,
		"",
		nil,
	)
	assert.NoError(t, err)
	_, present := config.ContextServers["helix-tasks"]
	assert.False(t, present)
}

func TestAgentToolsRevIsOrderIndependentAndSensitive(t *testing.T) {
	assert.Equal(t, AgentToolsRev([]string{"a", "b"}), AgentToolsRev([]string{"b", "a"}))
	assert.NotEqual(t, AgentToolsRev([]string{"a", "b"}), AgentToolsRev([]string{"a", "b", "c"}))
}
