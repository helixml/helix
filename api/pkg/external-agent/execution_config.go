package external_agent

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// MaterializeCodeAgentConfig copies the executable coding-agent fields out of
// a reusable App. The returned value is self-contained and safe to persist on
// a Project or SpecTask.
func MaterializeCodeAgentConfig(app *types.App, overrides *types.CodeAgentOverrides) (*types.CodeAgentExecutionConfig, error) {
	effective := ApplyCodeAgentOverrides(app, overrides)
	assistant := FindZedExternalAssistant(effective)
	if assistant == nil {
		return nil, fmt.Errorf("agent has no coding assistant")
	}

	runtime := assistant.CodeAgentRuntime
	if runtime == "" {
		runtime = types.CodeAgentRuntimeZedAgent
	}
	credentialType := assistant.CodeAgentCredentialType
	if credentialType == "" {
		credentialType = types.CodeAgentCredentialTypeAPIKey
	}
	provider, model := AssistantModelSelection(assistant)
	if credentialType.IsSubscription() {
		provider = ""
		if runtime == types.CodeAgentRuntimeClaudeCode {
			model = assistant.ClaudeSubscriptionModel
			if model == "" {
				model = "claude-opus-5"
			}
		}
	}

	config := &types.CodeAgentExecutionConfig{
		Runtime:            runtime,
		CredentialType:     credentialType,
		ProviderRef:        provider,
		Model:              model,
		ReasoningEffort:    assistant.ReasoningEffort,
		GooseRecipeRepoURL: assistant.GooseRecipeRepoURL,
		GooseRecipes:       append([]types.AssistantGooseRecipe(nil), assistant.GooseRecipes...),
	}
	if overrides != nil {
		config.ServiceTier = overrides.ServiceTier
	}
	return config, nil
}

// AppFromCodeAgentConfig adapts task-owned execution data to the existing Zed
// configuration generator. It does not represent a persisted App and must
// never be stored or exposed as an Agent identity.
func AppFromCodeAgentConfig(config *types.CodeAgentExecutionConfig, ownerID, organizationID string) *types.App {
	if config == nil {
		return nil
	}
	assistant := types.AssistantConfig{
		AgentType:               types.AgentTypeZedExternal,
		CodeAgentRuntime:        config.Runtime,
		CodeAgentCredentialType: config.CredentialType,
		ReasoningEffort:         config.ReasoningEffort,
		GooseRecipeRepoURL:      config.GooseRecipeRepoURL,
		GooseRecipes:            append([]types.AssistantGooseRecipe(nil), config.GooseRecipes...),
	}
	switch {
	case config.Runtime == types.CodeAgentRuntimeClaudeCode && config.CredentialType.IsSubscription():
		assistant.ClaudeSubscriptionModel = config.Model
	case config.Runtime == types.CodeAgentRuntimeClaudeCode:
		assistant.GenerationModelProvider = config.ProviderRef
		assistant.GenerationModel = config.Model
	default:
		assistant.Provider = config.ProviderRef
		assistant.Model = config.Model
	}
	return &types.App{
		Owner:          ownerID,
		OrganizationID: organizationID,
		Config: types.AppConfig{Helix: types.AppHelixConfig{
			ExternalAgentEnabled: true,
			Assistants:           []types.AssistantConfig{assistant},
		}},
	}
}

// ApplyExecutionOverrides is used only during legacy migration where an old
// task may already have a partially materialized config plus overrides.
func ApplyExecutionOverrides(config *types.CodeAgentExecutionConfig, overrides *types.CodeAgentOverrides) *types.CodeAgentExecutionConfig {
	if config == nil {
		return nil
	}
	effective := *config
	effective.GooseRecipes = append([]types.AssistantGooseRecipe(nil), config.GooseRecipes...)
	if overrides == nil {
		return &effective
	}
	if overrides.ProviderRef != "" {
		effective.ProviderRef = overrides.ProviderRef
	}
	if overrides.Model != "" {
		effective.Model = overrides.Model
	}
	if overrides.ReasoningEffort != "" {
		effective.ReasoningEffort = overrides.ReasoningEffort
	}
	effective.ServiceTier = overrides.ServiceTier
	if effective.CredentialType.IsSubscription() {
		effective.ProviderRef = ""
	}
	return &effective
}
