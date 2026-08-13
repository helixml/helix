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
			SpecTaskID:       "spt_123",
			CodeAgentRuntime: types.CodeAgentRuntimeOpenCode,
		},
	}, nil)

	attribution, err := server.resolveProxyUsageAttribution(context.Background(), user, "response_123")
	require.NoError(t, err)
	require.Equal(t, "ses_123", attribution.SessionID)
	require.Equal(t, "app_current", attribution.AppID)
	require.Equal(t, types.CodeAgentRuntimeOpenCode, attribution.CodeAgentRuntime)
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
