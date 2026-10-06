package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestResolveProxyUsageAttributionUsesCurrentSessionApp(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	user := &types.User{
		SessionID:      "ses_123",
		SpecTaskID:     "spt_123",
		OrganizationID: "org_123",
	}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_123").Return(&types.Session{
		ID:             "ses_123",
		OrganizationID: "org_123",
		ParentApp:      "app_current",
		Metadata: types.SessionMetadata{
			CodeAgentRuntime:   types.CodeAgentRuntimeOpenCode,
			CodeAgentOverrides: &types.CodeAgentOverrides{Model: "qwen3.8-27b"},
		},
	}, nil)

	attribution, err := server.resolveProxyUsageAttribution(context.Background(), user, "response_123")
	require.NoError(t, err)
	require.Equal(t, "ses_123", attribution.SessionID)
	require.Equal(t, "app_current", attribution.AppID)
	require.Equal(t, types.CodeAgentRuntimeOpenCode, attribution.CodeAgentRuntime)
	require.Equal(t, "qwen3.8-27b", attribution.CodeAgentOverrides.Model)
	require.False(t, attribution.hasExecutionConfig)
}

func TestResolveProxyUsageAttributionUsesAuthoritativeSpecTaskConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_123").Return(&types.Session{
		ID:             "ses_123",
		OrganizationID: "org_123",
		ParentApp:      "app_stale",
		Metadata: types.SessionMetadata{
			SpecTaskID:       "spt_123",
			CodeAgentRuntime: types.CodeAgentRuntimeOpenCode,
		},
	}, nil)
	mockStore.EXPECT().GetSpecTask(gomock.Any(), "spt_123").Return(&types.SpecTask{
		ID:                "spt_123",
		PlanningSessionID: "ses_123",
		HelixAppID:        "app_current",
		CodeAgentOverrides: &types.CodeAgentOverrides{
			ProviderRef:     "pe_qwen",
			Model:           "qwen3.8-27b",
			ReasoningEffort: "medium",
		},
	}, nil)

	attribution, err := server.resolveProxyUsageAttribution(context.Background(), &types.User{
		SessionID:      "ses_123",
		SpecTaskID:     "spt_123",
		OrganizationID: "org_123",
	}, "response_123")
	require.NoError(t, err)
	require.Equal(t, "app_current", attribution.AppID)
	require.Equal(t, "pe_qwen", attribution.CodeAgentOverrides.ProviderRef)
	require.Equal(t, "qwen3.8-27b", attribution.CodeAgentOverrides.Model)
	require.Equal(t, "medium", attribution.CodeAgentOverrides.ReasoningEffort)
	require.False(t, attribution.hasExecutionConfig)
}

func TestResolveProxyUsageAttributionMarksExecutionConfig(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_123").Return(&types.Session{
		ID: "ses_123",
		Metadata: types.SessionMetadata{CodeAgentConfig: &types.CodeAgentExecutionConfig{
			Runtime: types.CodeAgentRuntimeClaudeCode,
		}},
	}, nil)

	attribution, err := server.resolveProxyUsageAttribution(context.Background(), &types.User{
		SessionID: "ses_123",
	}, "")

	require.NoError(t, err)
	require.True(t, attribution.hasExecutionConfig)
}

func TestResolveProxyUsageAttributionLeavesOrdinaryAPIRequestUnchanged(t *testing.T) {
	server := &HelixAPIServer{}
	attribution, err := server.resolveProxyUsageAttribution(context.Background(), &types.User{AppID: "app_123"}, "response_123")
	require.NoError(t, err)
	require.Equal(t, "response_123", attribution.SessionID)
	require.Equal(t, "app_123", attribution.AppID)
}

func TestResolveProxyUsageAttributionRejectsTaskMismatch(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_123").Return(&types.Session{
		ID:       "ses_123",
		Metadata: types.SessionMetadata{SpecTaskID: "spt_other"},
	}, nil)

	_, err := server.resolveProxyUsageAttribution(context.Background(), &types.User{
		SessionID:  "ses_123",
		SpecTaskID: "spt_123",
	}, "response_123")
	require.ErrorContains(t, err, "different task")
}
