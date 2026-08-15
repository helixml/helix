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
