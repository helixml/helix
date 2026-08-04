package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	oai "github.com/helixml/helix/api/pkg/openai"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestPreflightHelixAgentModelsIgnoresTopLevelProvider(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	openAIClient := oai.NewMockClient(ctrl)
	openAIClient.EXPECT().BillingEnabled().Return(false).Times(1)

	providerManager := manager.NewMockProviderManager(ctrl)
	providerManager.EXPECT().
		GetClient(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *manager.GetClientRequest) (oai.Client, error) {
			require.Equal(t, "openai", req.Provider)
			return openAIClient, nil
		}).
		Times(1)

	c := &Controller{
		Options: Options{
			Config: &config.ServerConfig{},
		},
		providerManager: providerManager,
	}

	assistant := &types.AssistantConfig{
		Name:      "agent",
		AgentType: types.AgentTypeHelixAgent,
		Provider:  "user/google",
		Model:     "models/gemini-2.0-flash",

		ReasoningModelProvider:       "openai",
		ReasoningModel:               "gpt-5-nano",
		GenerationModelProvider:      "openai",
		GenerationModel:              "gpt-5-nano",
		SmallReasoningModelProvider:  "openai",
		SmallReasoningModel:          "gpt-5-nano",
		SmallGenerationModelProvider: "openai",
		SmallGenerationModel:         "gpt-5-nano",
	}

	err := c.preflightHelixAgentModels(context.Background(), &types.User{ID: "user1"}, &ChatCompletionOptions{}, assistant)
	require.NoError(t, err)
}

func TestRunAgentPassesPersonalProviderScope(t *testing.T) {
	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	providerManager.EXPECT().GetClient(gomock.Any(), &manager.GetClientRequest{
		Provider: "pe_personal",
		Owner:    "org_x",
		UserID:   "user_x",
	}).Return(nil, errors.New("stop after provider lookup"))

	c := &Controller{
		Options:         Options{Config: &config.ServerConfig{}},
		providerManager: providerManager,
	}
	ctx := oai.SetContextAppID(context.Background(), "app_x")
	_, _, err := c.runAgent(ctx, &runAgentRequest{
		OrganizationID: "org_x",
		Assistant: &types.AssistantConfig{
			ReasoningModelProvider: "pe_personal",
			ReasoningModel:         "model_x",
		},
		User: &types.User{ID: "user_x"},
	})
	require.ErrorContains(t, err, "stop after provider lookup")
}
