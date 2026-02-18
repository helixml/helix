package server

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func Test_getProviderEndpoint(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name   string
		user   *types.User
		cfg    *config.ServerConfig
		setup  func(mockStore *store.MockStore)
		assert func(t *testing.T, endpoint *types.ProviderEndpoint, err error)
	}{
		{
			name: "uses project assistant provider for database lookup",
			user: &types.User{
				ID:        "user-1",
				ProjectID: "project-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{
					ID:                "project-1",
					DefaultHelixAppID: "app-1",
				}, nil)
				mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{
					ID: "app-1",
					Config: types.AppConfig{
						Helix: types.AppHelixConfig{
							Assistants: []types.AssistantConfig{
								{Provider: "openai"},
							},
						},
					},
				}, nil)
				mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
					Owner: "",
					Name:  "openai",
				}).Return(&types.ProviderEndpoint{
					Name: "openai",
				}, nil)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.NoError(t, err)
				require.NotNil(t, endpoint)
				assert.Equal(t, "openai", endpoint.Name)
			},
		},
		{
			name: "returns error when project app has no assistants",
			user: &types.User{
				ID:        "user-1",
				ProjectID: "project-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{
					ID:                "project-1",
					DefaultHelixAppID: "app-1",
				}, nil)
				mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{
					ID: "app-1",
				}, nil)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.Error(t, err)
				assert.Nil(t, endpoint)
				assert.Contains(t, err.Error(), "no assistants found for project app")
			},
		},
		{
			name: "returns error when project app assistant provider is empty",
			user: &types.User{
				ID:        "user-1",
				ProjectID: "project-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{
					ID:                "project-1",
					DefaultHelixAppID: "app-1",
				}, nil)
				mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{
					ID: "app-1",
					Config: types.AppConfig{
						Helix: types.AppHelixConfig{
							Assistants: []types.AssistantConfig{
								{Provider: ""},
							},
						},
					},
				}, nil)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.Error(t, err)
				assert.Nil(t, endpoint)
				assert.Contains(t, err.Error(), "no provider found for project app")
			},
		},
		{
			name: "uses provider id query when provider has endpoint prefix",
			user: &types.User{
				ID:        "user-1",
				ProjectID: "project-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				endpointID := system.ProviderEndpointPrefix + "provider-123"

				mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{
					ID:                "project-1",
					DefaultHelixAppID: "app-1",
				}, nil)
				mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{
					ID: "app-1",
					Config: types.AppConfig{
						Helix: types.AppHelixConfig{
							Assistants: []types.AssistantConfig{
								{Provider: endpointID},
							},
						},
					},
				}, nil)
				mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
					Owner: "",
					ID:    endpointID,
				}).Return(&types.ProviderEndpoint{
					ID: endpointID,
				}, nil)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.NoError(t, err)
				require.NotNil(t, endpoint)
				assert.Equal(t, system.ProviderEndpointPrefix+"provider-123", endpoint.ID)
			},
		},
		{
			name: "uses organization provider endpoint when user is org member",
			user: &types.User{
				ID:             "user-1",
				OrganizationID: "org-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), &store.GetOrganizationMembershipQuery{
					OrganizationID: "org-1",
					UserID:         "user-1",
				}).Return(&types.OrganizationMembership{
					OrganizationID: "org-1",
					UserID:         "user-1",
					Role:           types.OrganizationRoleMember,
				}, nil)
				mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
					Owner: "org-1",
					Name:  "anthropic",
				}).Return(&types.ProviderEndpoint{
					Name:  "anthropic",
					Owner: "org-1",
				}, nil)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.NoError(t, err)
				require.NotNil(t, endpoint)
				assert.Equal(t, "org-1", endpoint.Owner)
				assert.Equal(t, "anthropic", endpoint.Name)
			},
		},
		{
			name: "falls back to built-in anthropic endpoint when database lookup fails",
			user: &types.User{
				ID: "user-1",
			},
			cfg: &config.ServerConfig{
				Providers: config.Providers{
					BillingEnabled: true,
					Anthropic: config.Anthropic{
						BaseURL: "https://api.anthropic.com/v1",
						APIKey:  "test-anthropic-key",
					},
				},
			},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
					Owner: "",
					Name:  "anthropic",
				}).Return(nil, store.ErrNotFound)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.NoError(t, err)
				require.NotNil(t, endpoint)
				assert.Equal(t, string(types.ProviderAnthropic), endpoint.ID)
				assert.Equal(t, string(types.ProviderAnthropic), endpoint.Name)
				assert.Equal(t, "https://api.anthropic.com/v1", endpoint.BaseURL)
				assert.Equal(t, "test-anthropic-key", endpoint.APIKey)
				assert.Equal(t, types.OwnerTypeSystem, endpoint.OwnerType)
				assert.Equal(t, string(types.OwnerTypeSystem), endpoint.Owner)
				assert.Equal(t, types.ProviderEndpointTypeGlobal, endpoint.EndpointType)
				assert.True(t, endpoint.BillingEnabled)
			},
		},
		{
			name: "returns error when provider is not found and built-in provider does not exist",
			user: &types.User{
				ID:        "user-1",
				ProjectID: "project-1",
			},
			cfg: &config.ServerConfig{},
			setup: func(mockStore *store.MockStore) {
				mockStore.EXPECT().GetProject(gomock.Any(), "project-1").Return(&types.Project{
					ID:                "project-1",
					DefaultHelixAppID: "app-1",
				}, nil)
				mockStore.EXPECT().GetApp(gomock.Any(), "app-1").Return(&types.App{
					ID: "app-1",
					Config: types.AppConfig{
						Helix: types.AppHelixConfig{
							Assistants: []types.AssistantConfig{
								{Provider: "openai"},
							},
						},
					},
				}, nil)
				mockStore.EXPECT().GetProviderEndpoint(gomock.Any(), &store.GetProviderEndpointsQuery{
					Owner: "",
					Name:  "openai",
				}).Return(nil, store.ErrNotFound)
			},
			assert: func(t *testing.T, endpoint *types.ProviderEndpoint, err error) {
				require.Error(t, err)
				assert.Nil(t, endpoint)
				assert.Contains(t, err.Error(), `provider "openai" not found`)
			},
		},
	}

	for _, testCase := range testCases {
		testCase := testCase

		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockStore := store.NewMockStore(ctrl)
			testCase.setup(mockStore)

			server := &HelixAPIServer{
				Cfg:   testCase.cfg,
				Store: mockStore,
			}

			endpoint, err := server.getProviderEndpoint(context.Background(), testCase.user)
			testCase.assert(t, endpoint, err)
		})
	}
}

func Test_parseAnthropicRequestModel(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		want  string
		found bool
	}{
		{
			name:  "valid model",
			body:  []byte(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}]}`),
			want:  "claude-sonnet-4-20250514",
			found: true,
		},
		{
			name:  "missing model",
			body:  []byte(`{"messages":[{"role":"user","content":"hi"}]}`),
			want:  "",
			found: false,
		},
		{
			name:  "invalid json",
			body:  []byte(`{"model":"claude-sonnet-4"`),
			want:  "",
			found: false,
		},
		{
			name:  "empty body",
			body:  []byte{},
			want:  "",
			found: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseAnthropicRequestModel(tt.body)
			assert.Equal(t, tt.found, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
