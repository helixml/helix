package external_agent

import "github.com/helixml/helix/api/pkg/types"

// ApplyCodeAgentOverrides returns an Agent copy with the SpecTask's model
// overrides applied to its effective zed_external assistant. The original
// reusable Agent is never mutated.
func ApplyCodeAgentOverrides(app *types.App, overrides *types.CodeAgentOverrides) *types.App {
	if app == nil || overrides == nil {
		return app
	}

	effective := *app
	effective.Config = app.Config
	effective.Config.Helix = app.Config.Helix
	effective.Config.Helix.Assistants = append([]types.AssistantConfig(nil), app.Config.Helix.Assistants...)

	assistant := FindZedExternalAssistant(&effective)
	if assistant == nil {
		return &effective
	}

	if overrides.ProviderRef != "" {
		assistant.Provider = overrides.ProviderRef
		assistant.GenerationModelProvider = overrides.ProviderRef
	}
	if overrides.Model != "" {
		switch {
		case assistant.CodeAgentRuntime == types.CodeAgentRuntimeClaudeCode && assistant.CodeAgentCredentialType.IsSubscription():
			assistant.ClaudeSubscriptionModel = overrides.Model
		case assistant.CodeAgentRuntime == types.CodeAgentRuntimeClaudeCode:
			assistant.GenerationModel = overrides.Model
		default:
			assistant.Model = overrides.Model
		}
	}
	if overrides.ReasoningEffort != "" {
		assistant.ReasoningEffort = overrides.ReasoningEffort
	}

	return &effective
}
