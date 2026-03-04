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
	SystemSettings *types.SystemSettings
}

func NewOptimusAgentApp(cfg OptimusConfig) *types.App {
	appName := "Optimus (" + cfg.ProjectName + ")"

	defaultAssistant := cfg.DefaultApp.Config.Helix.Assistants[0]

	assistant := types.AssistantConfig{
		Name:     appName,
		Provider: defaultAssistant.Provider,
		Model:    defaultAssistant.Model,

		ReasoningModelProvider: cfg.SystemSettings.OptimusReasoningModelProvider,
		ReasoningModel:         cfg.SystemSettings.OptimusReasoningModel,
		ReasoningModelEffort:   cfg.SystemSettings.OptimusReasoningModelEffort,

		GenerationModelProvider: cfg.SystemSettings.OptimusGenerationModelProvider,
		GenerationModel:         cfg.SystemSettings.OptimusGenerationModel,

		SmallReasoningModelProvider: cfg.SystemSettings.OptimusSmallReasoningModelProvider,
		SmallReasoningModel:         cfg.SystemSettings.OptimusSmallReasoningModel,
		SmallReasoningModelEffort:   cfg.SystemSettings.OptimusSmallReasoningModelEffort,

		SmallGenerationModelProvider: cfg.SystemSettings.OptimusSmallGenerationModelProvider,
		SmallGenerationModel:         cfg.SystemSettings.OptimusSmallGenerationModel,

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
				Description:      "Feel free to edit me and give me more skills!",
				Assistants:       []types.AssistantConfig{assistant},
				DefaultAgentType: types.AgentTypeHelixAgent,
			},
			Secrets:        make(map[string]string),
			AllowedDomains: []string{},
		},
	}
}
