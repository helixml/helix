package optimus

import (
	"github.com/helixml/helix/api/pkg/prompts/templates"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

type OptimusConfig struct {
	ProjectID      string
	ProjectName    string
	OrganizationID string
	OwnerID        string
	OwnerType      types.OwnerType
	DefaultApp     *types.App
}

func NewOptimusAgentApp(cfg OptimusConfig) *types.App {
	appName := "Optimus (" + cfg.ProjectName + ")"

	defaultAssistant := cfg.DefaultApp.Config.Helix.Assistants[0]

	assistant := types.AssistantConfig{
		Name:     appName,
		Provider: defaultAssistant.Provider,
		Model:    defaultAssistant.Model,

		ReasoningModelProvider: defaultAssistant.ReasoningModelProvider,
		ReasoningModel:         defaultAssistant.ReasoningModel,
		ReasoningModelEffort:   defaultAssistant.ReasoningModelEffort,

		GenerationModelProvider: defaultAssistant.GenerationModelProvider,
		GenerationModel:         defaultAssistant.GenerationModel,

		SmallReasoningModelProvider: defaultAssistant.SmallReasoningModelProvider,
		SmallReasoningModel:         defaultAssistant.SmallReasoningModel,
		SmallReasoningModelEffort:   defaultAssistant.SmallReasoningModelEffort,

		SmallGenerationModelProvider: defaultAssistant.SmallGenerationModelProvider,
		SmallGenerationModel:         defaultAssistant.SmallGenerationModel,

		AgentType:    types.AgentTypeHelixAgent,
		SystemPrompt: templates.OptimusTemplate,
		ProjectManager: types.AssistantProjectManager{
			Enabled:   true,
			ProjectID: cfg.ProjectID,
		},
	}

	return &types.App{
		ID:             system.GenerateAppID(),
		OrganizationID: cfg.OrganizationID,
		Owner:          cfg.OwnerID,
		OwnerType:      cfg.OwnerType,
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Name:             appName,
				Assistants:       []types.AssistantConfig{assistant},
				DefaultAgentType: types.AgentTypeHelixAgent,
			},
			Secrets:        make(map[string]string),
			AllowedDomains: []string{},
		},
	}
}
