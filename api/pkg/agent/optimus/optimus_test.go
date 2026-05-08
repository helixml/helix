package optimus

import (
	"testing"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

func TestNewOptimusAgentApp_UsesDefaultModelValuesWhenSystemSettingsAreNotProvided(t *testing.T) {
	defaultAssistant := types.AssistantConfig{
		Provider: "default-provider",
		Model:    "default-model",
	}
	defaultApp := &types.App{
		Config: types.AppConfig{
			Helix: types.AppHelixConfig{
				Assistants: []types.AssistantConfig{defaultAssistant},
			},
		},
	}

	tests := []struct {
		name           string
		systemSettings *types.SystemSettings
		expected       types.AssistantConfig
	}{
		{
			name:           "nil system settings",
			systemSettings: nil,
			expected: types.AssistantConfig{
				ReasoningModelProvider:       defaultAssistant.Provider,
				ReasoningModel:               defaultAssistant.Model,
				GenerationModelProvider:      defaultAssistant.Provider,
				GenerationModel:              defaultAssistant.Model,
				SmallReasoningModelProvider:  defaultAssistant.Provider,
				SmallReasoningModel:          defaultAssistant.Model,
				SmallGenerationModelProvider: defaultAssistant.Provider,
				SmallGenerationModel:         defaultAssistant.Model,
				ReasoningModelEffort:         "",
				SmallReasoningModelEffort:    "",
			},
		},
		{
			name:           "empty system settings",
			systemSettings: &types.SystemSettings{},
			expected: types.AssistantConfig{
				ReasoningModelProvider:       defaultAssistant.Provider,
				ReasoningModel:               defaultAssistant.Model,
				GenerationModelProvider:      defaultAssistant.Provider,
				GenerationModel:              defaultAssistant.Model,
				SmallReasoningModelProvider:  defaultAssistant.Provider,
				SmallReasoningModel:          defaultAssistant.Model,
				SmallGenerationModelProvider: defaultAssistant.Provider,
				SmallGenerationModel:         defaultAssistant.Model,
				ReasoningModelEffort:         "",
				SmallReasoningModelEffort:    "",
			},
		},
		{
			name: "all optimus model settings provided",
			systemSettings: &types.SystemSettings{
				OptimusReasoningModelProvider:       "reasoning-provider",
				OptimusReasoningModel:               "reasoning-model",
				OptimusReasoningModelEffort:         "high",
				OptimusGenerationModelProvider:      "generation-provider",
				OptimusGenerationModel:              "generation-model",
				OptimusSmallReasoningModelProvider:  "small-reasoning-provider",
				OptimusSmallReasoningModel:          "small-reasoning-model",
				OptimusSmallReasoningModelEffort:    "medium",
				OptimusSmallGenerationModelProvider: "small-generation-provider",
				OptimusSmallGenerationModel:         "small-generation-model",
			},
			expected: types.AssistantConfig{
				ReasoningModelProvider:       "reasoning-provider",
				ReasoningModel:               "reasoning-model",
				GenerationModelProvider:      "generation-provider",
				GenerationModel:              "generation-model",
				SmallReasoningModelProvider:  "small-reasoning-provider",
				SmallReasoningModel:          "small-reasoning-model",
				SmallGenerationModelProvider: "small-generation-provider",
				SmallGenerationModel:         "small-generation-model",
				ReasoningModelEffort:         "high",
				SmallReasoningModelEffort:    "medium",
			},
		},
		{
			name: "partial optimus model settings provided",
			systemSettings: &types.SystemSettings{
				OptimusReasoningModelProvider:     "reasoning-provider",
				OptimusSmallReasoningModelEffort: "low",
			},
			expected: types.AssistantConfig{
				ReasoningModelProvider:       "reasoning-provider",
				ReasoningModel:               defaultAssistant.Model,
				GenerationModelProvider:      defaultAssistant.Provider,
				GenerationModel:              defaultAssistant.Model,
				SmallReasoningModelProvider:  defaultAssistant.Provider,
				SmallReasoningModel:          defaultAssistant.Model,
				SmallGenerationModelProvider: defaultAssistant.Provider,
				SmallGenerationModel:         defaultAssistant.Model,
				ReasoningModelEffort:         "",
				SmallReasoningModelEffort:    "low",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := NewOptimusAgentApp(OptimusConfig{
				ProjectID:      "project-1",
				ProjectName:    "Test Project",
				OrganizationID: "org-1",
				OwnerID:        "user-1",
				OwnerType:      types.OwnerTypeUser,
				DefaultApp:     defaultApp,
				SystemSettings: tc.systemSettings,
			})

			require.NotNil(t, app)
			require.Len(t, app.Config.Helix.Assistants, 1)

			assistant := app.Config.Helix.Assistants[0]
			require.Equal(t, tc.expected.ReasoningModelProvider, assistant.ReasoningModelProvider)
			require.Equal(t, tc.expected.ReasoningModel, assistant.ReasoningModel)
			require.Equal(t, tc.expected.ReasoningModelEffort, assistant.ReasoningModelEffort)
			require.Equal(t, tc.expected.GenerationModelProvider, assistant.GenerationModelProvider)
			require.Equal(t, tc.expected.GenerationModel, assistant.GenerationModel)
			require.Equal(t, tc.expected.SmallReasoningModelProvider, assistant.SmallReasoningModelProvider)
			require.Equal(t, tc.expected.SmallReasoningModel, assistant.SmallReasoningModel)
			require.Equal(t, tc.expected.SmallReasoningModelEffort, assistant.SmallReasoningModelEffort)
			require.Equal(t, tc.expected.SmallGenerationModelProvider, assistant.SmallGenerationModelProvider)
			require.Equal(t, tc.expected.SmallGenerationModel, assistant.SmallGenerationModel)
		})
	}
}
