package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Creating a first org applies admin-stashed grants and saves the user row.
// It must save the stored row, not the request user, which carries the
// caller's bearer token and request-scoped admin flag.
func TestCreateOrganization_FirstOrgGrantDoesNotPersistRequestCredentials(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)

	plan := "pro"
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{Name: "acme"}).Return(nil, store.ErrNotFound)
	mockStore.EXPECT().CreateOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_1", Name: "acme"}, nil)
	mockStore.EXPECT().CreateOrganizationMembership(gomock.Any(), gomock.Any()).Return(&types.OrganizationMembership{}, nil)
	mockStore.EXPECT().CreateRole(gomock.Any(), gomock.Any()).Return(&types.Role{}, nil).AnyTimes()
	mockStore.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "usr_1"}).Return(&types.User{
		ID:             "usr_1",
		PlanOnFirstOrg: &plan,
		Token:          "hl-token-saved-by-an-earlier-request",
	}, nil)
	mockStore.EXPECT().GetOrganization(gomock.Any(), &store.GetOrganizationQuery{ID: "org_1"}).Return(&types.Organization{ID: "org_1"}, nil)
	mockStore.EXPECT().GetWalletByOrg(gomock.Any(), "org_1").Return(&types.Wallet{ID: "wal_1"}, nil)
	mockStore.EXPECT().UpdateWallet(gomock.Any(), gomock.Any()).Return(&types.Wallet{ID: "wal_1"}, nil)
	mockStore.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, u *types.User) (*types.User, error) {
		require.Empty(t, u.Token, "the bearer token must not be written to the users table")
		require.Empty(t, u.TokenType)
		require.False(t, u.Admin, "the request-scoped admin flag must not be persisted")
		require.Nil(t, u.PlanOnFirstOrg, "the grant is still consumed")
		return u, nil
	})

	cfg := &config.ServerConfig{}
	cfg.Organizations.CreateEnabledForNonAdmins = true
	server := &HelixAPIServer{Cfg: cfg, Store: mockStore}

	req := httptest.NewRequest(http.MethodPost, "/api/v1/organizations", strings.NewReader(`{"name":"acme"}`))
	req = req.WithContext(setRequestUser(context.Background(), types.User{
		ID:             "usr_1",
		Token:          "hl-live-request-token",
		TokenType:      types.TokenTypeAPIKey,
		Admin:          true,
		PlanOnFirstOrg: &plan,
	}))
	rr := httptest.NewRecorder()
	server.createOrganization(rr, req)

	require.Equal(t, http.StatusCreated, rr.Code, rr.Body.String())
}

// Member listings preload each member's user row; a token stored there must
// not reach other members.
func TestListOrganizationMembers_ScrubsStoredTokens(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).Return(&types.Organization{ID: "org_1"}, nil).AnyTimes()
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_bob", Role: types.OrganizationRoleMember}, nil).AnyTimes()
	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), gomock.Any()).Return([]*types.OrganizationMembership{{
		OrganizationID: "org_1",
		UserID:         "usr_alice",
		User:           types.User{ID: "usr_alice", Email: "alice@example.com", Token: "hl-alice-stored-token", TokenType: types.TokenTypeAPIKey},
	}}, nil)
	mockStore.EXPECT().ListOrganizationInvitations(gomock.Any(), gomock.Any()).Return(nil, nil)

	server := &HelixAPIServer{Cfg: &config.ServerConfig{}, Store: mockStore}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/organizations/org_1/members", nil)
	req = req.WithContext(setRequestUser(context.Background(), types.User{ID: "usr_bob"}))
	req = mux.SetURLVars(req, map[string]string{"id": "org_1"})
	rr := httptest.NewRecorder()
	server.listOrganizationMembers(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	require.Contains(t, rr.Body.String(), "alice@example.com")
	require.NotContains(t, rr.Body.String(), "hl-alice-stored-token")
}
