package logger

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func TestUsageLogger_CreateLLMCall_SetsAllIDs(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	logger := NewUsageLogger(mockStore)

	call := &types.LLMCall{
		OrganizationID:   "org_123",
		AppID:            "app_456",
		InteractionID:    "interaction_789",
		SpecTaskID:       "spec_task_101",
		ProjectID:        "project_202",
		UserID:           "user_303",
		Model:            "gpt-4",
		Provider:         "openai",
		PromptTokens:     100,
		CompletionTokens: 50,
	}

	mockStore.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.Equal(t, "org_123", metric.OrganizationID)
			assert.Equal(t, "app_456", metric.AppID)
			assert.Equal(t, "interaction_789", metric.InteractionID)
			assert.Equal(t, "spec_task_101", metric.SpecTaskID)
			assert.Equal(t, "project_202", metric.ProjectID)
			assert.Equal(t, "user_303", metric.UserID)
			assert.Equal(t, "gpt-4", metric.Model)
			assert.Equal(t, "openai", metric.Provider)
			assert.Equal(t, 100, metric.PromptTokens)
			assert.Equal(t, 50, metric.CompletionTokens)
			assert.Equal(t, 150, metric.TotalTokens)
			return metric, nil
		},
	)

	result, err := logger.CreateLLMCall(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, call, result)
}

func TestUsageLogger_CreateLLMCall_WithOptionalFieldsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	logger := NewUsageLogger(mockStore)

	call := &types.LLMCall{
		OrganizationID: "org_123",
		AppID:          "app_456",
		InteractionID:  "interaction_789",
		UserID:         "user_303",
		Model:          "gpt-4",
		Provider:       "openai",
	}

	mockStore.EXPECT().CreateUsageMetric(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, metric *types.UsageMetric) (*types.UsageMetric, error) {
			assert.Equal(t, "org_123", metric.OrganizationID)
			assert.Equal(t, "app_456", metric.AppID)
			assert.Equal(t, "interaction_789", metric.InteractionID)
			assert.Empty(t, metric.SpecTaskID)
			assert.Empty(t, metric.ProjectID)
			return metric, nil
		},
	)

	result, err := logger.CreateLLMCall(context.Background(), call)
	require.NoError(t, err)
	assert.Equal(t, call, result)
}
