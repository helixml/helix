package server

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAdminCreateServiceAccount(t *testing.T) {
	admin := &types.User{ID: "admin-123", Admin: true}

	tests := []struct {
		name           string
		caller         *types.User
		body           string
		setupMocks     func(*store.MockStore)
		expectedStatus int
		expectedError  string
		expectedOwner  string
	}{
		{
			name:   "creates a service user that owns a new organization",
			caller: admin,
			body:   `{"organization_name":"ws-acme","display_name":"Acme Recruiting"}`,
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "ws-acme"}).
					Return(nil, store.ErrNotFound)
				m.EXPECT().CreateUser(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
						assert.Equal(t, types.AuthProviderService, u.AuthProvider)
						assert.False(t, u.Admin)
						assert.Empty(t, u.Password)
						assert.Contains(t, u.Email, "@service.invalid")
						u.ID = "svc-1"
						return u, nil
					})
				m.EXPECT().CreateOrganization(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, o *types.Organization) (*types.Organization, error) {
						assert.Equal(t, "svc-1", o.Owner)
						o.ID = "org-1"
						return o, nil
					})
				m.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, mem *types.OrganizationMembership) (*types.OrganizationMembership, error) {
						assert.Equal(t, types.OrganizationRoleOwner, mem.Role)
						assert.Equal(t, "svc-1", mem.UserID)
						return mem, nil
					})
				m.EXPECT().CreateRole(gomock.Any(), gomock.Any()).Return(&types.Role{}, nil).AnyTimes()
			},
			expectedStatus: http.StatusOK,
			expectedOwner:  "svc-1",
		},
		{
			name:   "asking again returns the same account",
			caller: admin,
			body:   `{"organization_name":"ws-acme"}`,
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "ws-acme"}).
					Return(&types.Organization{ID: "org-1", Name: "ws-acme", Owner: "svc-1"}, nil)
				m.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "svc-1"}).
					Return(&types.User{ID: "svc-1", AuthProvider: types.AuthProviderService}, nil)
			},
			expectedStatus: http.StatusOK,
			expectedOwner:  "svc-1",
		},
		{
			name:   "will not hand over an organization a person owns",
			caller: admin,
			body:   `{"organization_name":"acme"}`,
			setupMocks: func(m *store.MockStore) {
				m.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "acme"}).
					Return(&types.Organization{ID: "org-2", Name: "acme", Owner: "person-1"}, nil)
				m.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "person-1"}).
					Return(&types.User{ID: "person-1", AuthProvider: types.AuthProviderOIDC}, nil)
			},
			expectedStatus: http.StatusConflict,
			expectedError:  "not owned by a service account",
		},
		{
			name:           "non-admin is refused",
			caller:         &types.User{ID: "user-789"},
			body:           `{"organization_name":"ws-acme"}`,
			setupMocks:     func(_ *store.MockStore) {},
			expectedStatus: http.StatusForbidden,
			expectedError:  "only admins",
		},
		{
			name:           "organization name is required",
			caller:         admin,
			body:           `{}`,
			setupMocks:     func(_ *store.MockStore) {},
			expectedStatus: http.StatusBadRequest,
			expectedError:  "organization_name is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockStore := store.NewMockStore(ctrl)
			tt.setupMocks(mockStore)
			server := &HelixAPIServer{Store: mockStore}

			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/service-accounts", bytes.NewBufferString(tt.body))
			req = req.WithContext(setTestRequestUser(req.Context(), tt.caller))

			result, httpErr := server.adminCreateServiceAccount(httptest.NewRecorder(), req)
			if tt.expectedError != "" {
				require.NotNil(t, httpErr)
				assert.Contains(t, httpErr.Error(), tt.expectedError)
				assert.Equal(t, tt.expectedStatus, httpErr.StatusCode)
				return
			}
			require.Nil(t, httpErr)
			assert.Equal(t, tt.expectedOwner, result.User.ID)
			assert.Equal(t, tt.expectedOwner, result.Organization.Owner)
		})
	}
}

func TestWalletHasPaidPlan(t *testing.T) {
	assert.True(t, (&types.Wallet{PlanOverride: types.PlanOverridePro}).HasPaidPlan())
	assert.False(t, (&types.Wallet{PlanOverride: types.PlanOverrideFree, SubscriptionStatus: "active"}).HasPaidPlan())
	assert.True(t, (&types.Wallet{SubscriptionStatus: "trialing"}).HasPaidPlan())
	assert.False(t, (&types.Wallet{SubscriptionStatus: "canceled"}).HasPaidPlan())
}
