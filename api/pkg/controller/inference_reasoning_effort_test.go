package controller

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestLoadAssistant_AppliesDirectChatReasoningEffort(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetUserMeta(gomock.Any(), "user-1").Return(&types.UserMeta{}, nil)

	c := &Controller{Options: Options{Store: mockStore}}
	assistant, err := c.loadAssistant(
		context.Background(),
		&types.User{ID: "user-1"},
		&ChatCompletionOptions{ReasoningEffort: "high"},
	)

	require.NoError(t, err)
	require.Equal(t, "high", assistant.ReasoningEffort)
}

func TestLoadAssistant_AppliesCodeAgentOverrides(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetAppWithTools(gomock.Any(), "app-1").Return(&types.App{
		ID: "app-1",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType:        types.AgentTypeZedExternal,
			CodeAgentRuntime: types.CodeAgentRuntimeOpenCode,
			Provider:         "pe-deepseek",
			Model:            "deepseek-v4-flash",
			ReasoningEffort:  types.ReasoningEffortNone,
		}}}},
	}, nil)
	mockStore.EXPECT().ListSecrets(gomock.Any(), gomock.Any()).Return(nil, nil)

	c := &Controller{Options: Options{Store: mockStore}}
	assistant, err := c.loadAssistant(
		context.Background(),
		&types.User{ID: "user-1"},
		&ChatCompletionOptions{
			AppID: "app-1",
			CodeAgentOverrides: &types.CodeAgentOverrides{
				ProviderRef:     "pe-qwen",
				Model:           "qwen3.8-27b",
				ReasoningEffort: "medium",
			},
		},
	)

	require.NoError(t, err)
	require.Equal(t, "pe-qwen", assistant.Provider)
	require.Equal(t, "qwen3.8-27b", assistant.Model)
	require.Equal(t, "medium", assistant.ReasoningEffort)
}
