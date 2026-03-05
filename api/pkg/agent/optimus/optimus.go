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
	systemSettings := cfg.SystemSettings
	if systemSettings == nil {
		systemSettings = &types.SystemSettings{}
	}
	reasoningModelProvider := systemSettings.OptimusReasoningModelProvider
	reasoningModel := systemSettings.OptimusReasoningModel
	generationModelProvider := systemSettings.OptimusGenerationModelProvider
	generationModel := systemSettings.OptimusGenerationModel
	smallReasoningModelProvider := systemSettings.OptimusSmallReasoningModelProvider
	smallReasoningModel := systemSettings.OptimusSmallReasoningModel
	smallGenerationModelProvider := systemSettings.OptimusSmallGenerationModelProvider
	smallGenerationModel := systemSettings.OptimusSmallGenerationModel

	if reasoningModelProvider == "" {
		reasoningModelProvider = defaultAssistant.Provider
	}
	if reasoningModel == "" {
		reasoningModel = defaultAssistant.Model
	}
	if generationModelProvider == "" {
		generationModelProvider = defaultAssistant.Provider
	}
	if generationModel == "" {
		generationModel = defaultAssistant.Model
	}
	if smallReasoningModelProvider == "" {
		smallReasoningModelProvider = defaultAssistant.Provider
	}
	if smallReasoningModel == "" {
		smallReasoningModel = defaultAssistant.Model
	}
	if smallGenerationModelProvider == "" {
		smallGenerationModelProvider = defaultAssistant.Provider
	}
	if smallGenerationModel == "" {
		smallGenerationModel = defaultAssistant.Model
	}

	assistant := types.AssistantConfig{
		Name:     appName,
		Provider: defaultAssistant.Provider,
		Model:    defaultAssistant.Model,

		ReasoningModelProvider: reasoningModelProvider,
		ReasoningModel:         reasoningModel,
		ReasoningModelEffort:   systemSettings.OptimusReasoningModelEffort,

		GenerationModelProvider: generationModelProvider,
		GenerationModel:         generationModel,

		SmallReasoningModelProvider: smallReasoningModelProvider,
		SmallReasoningModel:         smallReasoningModel,
		SmallReasoningModelEffort:   systemSettings.OptimusSmallReasoningModelEffort,

		SmallGenerationModelProvider: smallGenerationModelProvider,
		SmallGenerationModel:         smallGenerationModel,

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
