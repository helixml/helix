package external_agent

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestApplyCodeAgentOverrides(t *testing.T) {
	tests := []struct {
		name       string
		assistant  types.AssistantConfig
		overrides  *types.CodeAgentOverrides
		assertions func(*testing.T, types.AssistantConfig)
	}{
		{
			name: "codex subscription model and effort",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeCodexCLI,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
				Model:                   "gpt-old",
			},
			overrides: &types.CodeAgentOverrides{Model: "gpt-new", ReasoningEffort: "high", ServiceTier: "fast"},
			assertions: func(t *testing.T, got types.AssistantConfig) {
				require.Equal(t, "gpt-new", got.Model)
				require.Equal(t, "high", got.ReasoningEffort)
			},
		},
		{
			name: "claude subscription model",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
				ClaudeSubscriptionModel: "claude-old",
			},
			overrides: &types.CodeAgentOverrides{Model: "claude-new"},
			assertions: func(t *testing.T, got types.AssistantConfig) {
				require.Equal(t, "claude-new", got.ClaudeSubscriptionModel)
			},
		},
		{
			name: "claude api provider and generation model",
			assistant: types.AssistantConfig{
				AgentType:               types.AgentTypeZedExternal,
				CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
				CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
				GenerationModelProvider: "old-provider",
				GenerationModel:         "claude-old",
			},
			overrides: &types.CodeAgentOverrides{ProviderRef: "new-provider", Model: "claude-new"},
			assertions: func(t *testing.T, got types.AssistantConfig) {
				require.Equal(t, "new-provider", got.GenerationModelProvider)
				require.Equal(t, "claude-new", got.GenerationModel)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{tt.assistant},
			}}}
			got := ApplyCodeAgentOverrides(app, tt.overrides)
			require.NotSame(t, app, got)
			tt.assertions(t, got.Config.Helix.Assistants[0])
			require.Equal(t, tt.assistant, app.Config.Helix.Assistants[0], "base app must not be mutated")
		})
	}
}

func TestSandboxResourceOverridesValidPreset(t *testing.T) {
	require.True(t, (types.SandboxResourceOverrides{VCPUs: 1, MemoryMB: 2048}).ValidPreset())
	require.True(t, (types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 8192}).ValidPreset())
	require.True(t, (types.SandboxResourceOverrides{VCPUs: 8, MemoryMB: 16384}).ValidPreset())
	require.False(t, (types.SandboxResourceOverrides{VCPUs: 4, MemoryMB: 4096}).ValidPreset())
	require.False(t, (types.SandboxResourceOverrides{}).ValidPreset())
}

func TestEffectiveSpecTaskSandboxResourcesUsesDefault(t *testing.T) {
	resources := types.EffectiveSpecTaskSandboxResources(nil)
	require.Equal(t, types.DefaultSpecTaskSandboxVCPUs, resources.VCPUs)
	require.Equal(t, types.DefaultSpecTaskSandboxMemoryMB, resources.MemoryMB)
}
