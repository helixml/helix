package server

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func deferredZedAgent() *types.Agent {
	app := &types.Agent{}
	app.Config.Helix.DefaultAgentType = types.AgentTypeZedExternal
	app.Config.Helix.Assistants = []types.AssistantConfig{{
		Name:             "Zed Agent",
		AgentType:        types.AgentTypeZedExternal,
		CodeAgentRuntime: types.CodeAgentRuntimeZedAgent,
	}}
	return app
}

func TestApplyDefaultNewProjectAgentConfig(t *testing.T) {
	app := deferredZedAgent()
	settings := &types.SystemSettings{
		DefaultNewProjectAgentProvider:        "pe_default",
		DefaultNewProjectAgentModel:           "gpt-5.6",
		DefaultNewProjectAgentReasoningEffort: types.ReasoningEffortHigh,
	}

	err := applyDefaultNewProjectAgentConfig(settings, app)
	require.NoError(t, err)

	assistant := app.Config.Helix.Assistants[0]
	require.Equal(t, "pe_default", assistant.Provider)
	require.Equal(t, "gpt-5.6", assistant.Model)
	require.Equal(t, types.ReasoningEffortHigh, assistant.ReasoningEffort)
	require.Equal(t, types.CodeAgentCredentialTypeAPIKey, assistant.CodeAgentCredentialType)
}

func TestApplyDefaultNewProjectAgentConfigDefaultsReasoningEffort(t *testing.T) {
	app := deferredZedAgent()

	err := applyDefaultNewProjectAgentConfig(&types.SystemSettings{
		DefaultNewProjectAgentProvider: "pe_default",
		DefaultNewProjectAgentModel:    "gpt-5.6",
	}, app)
	require.NoError(t, err)
	require.Equal(t, types.ReasoningEffortNone, app.Config.Helix.Assistants[0].ReasoningEffort)
}

func TestApplyDefaultNewProjectAgentConfigPreservesExplicitConfig(t *testing.T) {
	app := deferredZedAgent()
	app.Config.Helix.Assistants[0].Provider = "pe_explicit"
	app.Config.Helix.Assistants[0].Model = "explicit-model"
	app.Config.Helix.Assistants[0].ReasoningEffort = types.ReasoningEffortLow

	err := applyDefaultNewProjectAgentConfig(&types.SystemSettings{
		DefaultNewProjectAgentProvider:        "pe_default",
		DefaultNewProjectAgentModel:           "default-model",
		DefaultNewProjectAgentReasoningEffort: types.ReasoningEffortHigh,
	}, app)
	require.NoError(t, err)
	require.Equal(t, "pe_explicit", app.Config.Helix.Assistants[0].Provider)
	require.Equal(t, "explicit-model", app.Config.Helix.Assistants[0].Model)
	require.Equal(t, types.ReasoningEffortLow, app.Config.Helix.Assistants[0].ReasoningEffort)
}

func TestApplyDefaultNewProjectAgentConfigPreservesSubscriptionConfig(t *testing.T) {
	app := deferredZedAgent()
	app.Config.Helix.Assistants[0].CodeAgentRuntime = types.CodeAgentRuntimeClaudeCode
	app.Config.Helix.Assistants[0].CodeAgentCredentialType = types.CodeAgentCredentialTypeSubscription

	err := applyDefaultNewProjectAgentConfig(&types.SystemSettings{
		DefaultNewProjectAgentProvider: "pe_default",
		DefaultNewProjectAgentModel:    "default-model",
	}, app)
	require.NoError(t, err)
	require.Empty(t, app.Config.Helix.Assistants[0].Provider)
	require.Empty(t, app.Config.Helix.Assistants[0].Model)
}

func TestApplyDefaultNewProjectAgentConfigDefersNativeHarnessModelSelection(t *testing.T) {
	for _, runtime := range []types.CodeAgentRuntime{
		types.CodeAgentRuntimeClaudeCode,
		types.CodeAgentRuntimeCodexCLI,
	} {
		t.Run(string(runtime), func(t *testing.T) {
			app := deferredZedAgent()
			app.Config.Helix.Assistants[0].CodeAgentRuntime = runtime

			err := applyDefaultNewProjectAgentConfig(&types.SystemSettings{
				DefaultNewProjectAgentProvider: "pe_default",
				DefaultNewProjectAgentModel:    "qwen3.8-27b",
			}, app)
			require.NoError(t, err)
			require.Empty(t, app.Config.Helix.Assistants[0].Provider)
			require.Empty(t, app.Config.Helix.Assistants[0].Model)
			require.Empty(t, app.Config.Helix.Assistants[0].CodeAgentCredentialType)
		})
	}
}

func TestApplyDefaultNewProjectAgentConfigRequiresConfiguredDefaults(t *testing.T) {
	err := applyDefaultNewProjectAgentConfig(&types.SystemSettings{}, deferredZedAgent())
	require.Error(t, err)
	require.Contains(t, err.Error(), "Admin > System Settings")
}
