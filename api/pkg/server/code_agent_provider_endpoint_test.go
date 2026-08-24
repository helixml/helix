package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/openai/manager"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestCodeAgentProviderSelectionKeepsLegacyAppFallbackWithModelOverride(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	server := &HelixAPIServer{Store: mockStore}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{
		ID:        "ses_1",
		ParentApp: "app_1",
		Metadata: types.SessionMetadata{
			CodeAgentOverrides: &types.CodeAgentOverrides{Model: "claude-opus-4-8"},
		},
	}, nil)
	mockStore.EXPECT().GetApp(gomock.Any(), "app_1").Return(&types.App{
		ID: "app_1",
		Config: types.AppConfig{Helix: types.AppHelixConfig{Assistants: []types.AssistantConfig{{
			AgentType: types.AgentTypeZedExternal,
			Provider:  "anthropic",
			Model:     "claude-opus-4-8",
		}}}},
	}, nil)

	selection, err := server.codeAgentProviderSelection(context.Background(), &types.User{
		ID:        "user_1",
		SessionID: "ses_1",
	})

	require.NoError(t, err)
	require.Equal(t, "anthropic", selection.ProviderRef)
}

func TestResolveCodeAgentProviderEndpointResolvesSyntheticGlobalOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	synthetic := &types.ProviderEndpoint{ID: "global/togetherai", Name: "togetherai", EndpointType: types.ProviderEndpointTypeGlobal}
	providerManager.EXPECT().
		ListProviderEndpoints(gomock.Any(), "").
		Return([]*types.ProviderEndpoint{synthetic}, nil)

	server := &HelixAPIServer{
		Cfg: &config.ServerConfig{Providers: config.Providers{TogetherAI: config.TogetherAI{
			APIKey: "global-key",
		}}},
		providerManager: providerManager,
	}
	endpoint, err := server.resolveCodeAgentProviderEndpoint(context.Background(), &types.User{
		ID:             "user_1",
		OrganizationID: "org_1",
	}, "global/togetherai")

	require.NoError(t, err)
	require.Same(t, synthetic, endpoint)
}

func TestResolveCodeAgentProviderEndpointPrefersLegacyNamedOrganizationAnthropic(t *testing.T) {
	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	orgEndpoint := &types.ProviderEndpoint{
		ID:           "pe_org_anthropic",
		Name:         "user/anthropic",
		EndpointType: types.ProviderEndpointTypeOrg,
	}
	providerManager.EXPECT().
		ListProviderEndpointsForOwner(gomock.Any(), "org_1", types.OwnerTypeOrg).
		Return([]*types.ProviderEndpoint{
			{Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal},
			orgEndpoint,
		}, nil)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, providerManager: providerManager}
	endpoint, err := server.resolveCodeAgentProviderEndpoint(context.Background(), &types.User{
		ID: "user_1", OrganizationID: "org_1",
	}, "anthropic")

	require.NoError(t, err)
	require.Same(t, orgEndpoint, endpoint)
}

func TestResolveCodeAgentProviderEndpointKeepsExplicitDatabaseID(t *testing.T) {
	ctrl := gomock.NewController(t)
	providerManager := manager.NewMockProviderManager(ctrl)
	globalEndpoint := &types.ProviderEndpoint{ID: "pe_global_anthropic", Name: "anthropic", EndpointType: types.ProviderEndpointTypeGlobal}
	providerManager.EXPECT().
		ListProviderEndpointsForOwner(gomock.Any(), "org_1", types.OwnerTypeOrg).
		Return([]*types.ProviderEndpoint{
			globalEndpoint,
			{ID: "pe_org_anthropic", Name: "user/anthropic", EndpointType: types.ProviderEndpointTypeOrg},
		}, nil)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, providerManager: providerManager}
	endpoint, err := server.resolveCodeAgentProviderEndpoint(context.Background(), &types.User{
		ID: "user_1", OrganizationID: "org_1",
	}, "pe_global_anthropic")

	require.NoError(t, err)
	require.Same(t, globalEndpoint, endpoint)
}
