package external_agent

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestMaterializeCodeAgentConfig(t *testing.T) {
	app := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
		AgentType:               types.AgentTypeZedExternal,
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
		GenerationModelProvider: "provider-old",
		GenerationModel:         "model-old",
		ReasoningEffort:         "low",
		GooseRecipeRepoURL:      "https://example.test/recipes.git",
		GooseRecipes:            []types.AssistantGooseRecipe{{Name: "review", Path: "review.yaml"}},
	}}}}}

	got, err := MaterializeCodeAgentConfig(app, &types.CodeAgentOverrides{
		ProviderRef: "provider-new", ReasoningEffort: "high", ServiceTier: "fast",
	})
	require.NoError(t, err)
	require.Equal(t, types.CodeAgentRuntimeClaudeCode, got.Runtime)
	require.Equal(t, types.CodeAgentCredentialTypeAPIKey, got.CredentialType)
	require.Equal(t, "provider-new", got.ProviderRef)
	require.Equal(t, "model-old", got.Model)
	require.Equal(t, "high", got.ReasoningEffort)
	require.Equal(t, "fast", got.ServiceTier)
	require.Len(t, got.GooseRecipes, 1)
}

func TestMaterializeClaudeSubscriptionModel(t *testing.T) {
	app := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
		AgentType:               types.AgentTypeZedExternal,
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
		ClaudeSubscriptionModel: "claude-opus-5",
	}}}}}

	got, err := MaterializeCodeAgentConfig(app, &types.CodeAgentOverrides{Model: "claude-sonnet-5"})
	require.NoError(t, err)
	require.Empty(t, got.ProviderRef)
	require.Equal(t, "claude-sonnet-5", got.Model)

	roundTrip := AppFromCodeAgentConfig(got, "user-1", "org-1")
	assistant := FindZedExternalAssistant(roundTrip)
	require.Equal(t, "claude-sonnet-5", assistant.ClaudeSubscriptionModel)
}

func TestMaterializeClaudeSubscriptionModelDefaultsToOpus(t *testing.T) {
	app := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
		AgentType:               types.AgentTypeZedExternal,
		CodeAgentRuntime:        types.CodeAgentRuntimeClaudeCode,
		CodeAgentCredentialType: types.CodeAgentCredentialTypeSubscription,
	}}}}}

	got, err := MaterializeCodeAgentConfig(app, nil)
	require.NoError(t, err)
	require.Equal(t, "claude-opus-5", got.Model)
}

func TestApplyCodeAgentExecutionConfigPreservesAgentBehavior(t *testing.T) {
	app := &types.App{Config: types.AppConfig{Helix: types.AppHelixConfig{
		Name: "org bot",
		Assistants: []types.AssistantConfig{{
			AgentType:               types.AgentTypeZedExternal,
			SystemPrompt:            "keep these instructions",
			CodeAgentRuntime:        types.CodeAgentRuntimeZedAgent,
			CodeAgentCredentialType: types.CodeAgentCredentialTypeAPIKey,
			Provider:                "provider-old",
			Model:                   "model-old",
		}},
	}}}

	effective := ApplyCodeAgentExecutionConfig(app, &types.CodeAgentExecutionConfig{
		Runtime:        types.CodeAgentRuntimeCodexCLI,
		CredentialType: types.CodeAgentCredentialTypeSubscription,
		Model:          "gpt-5.6-sol",
	})
	assistant := FindZedExternalAssistant(effective)
	require.Equal(t, "keep these instructions", assistant.SystemPrompt)
	require.Equal(t, types.CodeAgentRuntimeCodexCLI, assistant.CodeAgentRuntime)
	require.Equal(t, types.CodeAgentCredentialTypeSubscription, assistant.CodeAgentCredentialType)
	require.Equal(t, "gpt-5.6-sol", assistant.Model)
	require.Empty(t, assistant.Provider)

	original := FindZedExternalAssistant(app)
	require.Equal(t, types.CodeAgentRuntimeZedAgent, original.CodeAgentRuntime)
	require.Equal(t, "model-old", original.Model)
}

func TestApplyCodeAgentExecutionConfigAddsCodingAssistantToAppWithoutOne(t *testing.T) {
	app := &types.App{
		Owner:          "user-1",
		OrganizationID: "org-1",
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			Name: "plain session",
		}},
	}

	effective := ApplyCodeAgentExecutionConfig(app, &types.CodeAgentExecutionConfig{
		Runtime:        types.CodeAgentRuntimeOpenCode,
		CredentialType: types.CodeAgentCredentialTypeAPIKey,
		ProviderRef:    "provider-1",
		Model:          "model-1",
	})

	assistant := FindZedExternalAssistant(effective)
	require.NotNil(t, assistant)
	require.True(t, effective.Config.Helix.ExternalAgentEnabled)
	require.Equal(t, types.CodeAgentRuntimeOpenCode, assistant.CodeAgentRuntime)
	require.Equal(t, "provider-1", assistant.Provider)
	require.Equal(t, "model-1", assistant.Model)
	require.Empty(t, app.Config.Helix.Assistants)
}
