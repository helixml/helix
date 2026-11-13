package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// Regression tests for Helix Cyber finding TST-01-145: GET /api/v1/users/{id}
// returned any user's record to any authenticated caller, so one tenant's
// admin key could read users belonging only to another tenant.

const (
	tenantAOrg   = "org_testorg"
	tenantBOrg   = "org_testorg2"
	tenantAAdmin = "user-testorg-admin"
	tenantBAdmin = "user-testorg2-admin"
	tenantAUser  = "user-testorg-synthetic"
	tenantBUser  = "user-testorg2-synthetic"
)

func membershipsByUser() map[string][]*types.OrganizationMembership {
	m := func(user, org string, role types.OrganizationRole) *types.OrganizationMembership {
		return &types.OrganizationMembership{UserID: user, OrganizationID: org, Role: role}
	}
	return map[string][]*types.OrganizationMembership{
		tenantAAdmin: {m(tenantAAdmin, tenantAOrg, types.OrganizationRoleOwner)},
		tenantBAdmin: {m(tenantBAdmin, tenantBOrg, types.OrganizationRoleOwner)},
		tenantAUser:  {m(tenantAUser, tenantAOrg, types.OrganizationRoleMember)},
		tenantBUser:  {m(tenantBUser, tenantBOrg, types.OrganizationRoleMember)},
	}
}

func newUserDetailsTestServer(t *testing.T) *HelixAPIServer {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	all := membershipsByUser()

	mockStore.EXPECT().ListOrganizationMemberships(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, q *store.ListOrganizationMembershipsQuery) ([]*types.OrganizationMembership, error) {
			var out []*types.OrganizationMembership
			for _, m := range all[q.UserID] {
				if q.OrganizationID == "" || q.OrganizationID == m.OrganizationID {
					out = append(out, m)
				}
			}
			return out, nil
		}).AnyTimes()

	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, q *store.GetOrganizationMembershipQuery) (*types.OrganizationMembership, error) {
			for _, m := range all[q.UserID] {
				if m.OrganizationID == q.OrganizationID {
					return m, nil
				}
			}
			return nil, store.ErrNotFound
		}).AnyTimes()

	mockStore.EXPECT().GetOrganization(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, q *store.GetOrganizationQuery) (*types.Organization, error) {
			id := q.ID
			if id == "" {
				id = "org_" + q.Name
			}
			if id != tenantAOrg && id != tenantBOrg {
				return nil, store.ErrNotFound
			}
			return &types.Organization{ID: id, Name: strings.TrimPrefix(id, "org_")}, nil
		}).AnyTimes()

	mockStore.EXPECT().SearchUsers(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, q *store.SearchUsersQuery) ([]*types.User, int64, error) {
			var out []*types.User
			for userID, ms := range all {
				for _, m := range ms {
					if q.OrganizationID == "" || m.OrganizationID == q.OrganizationID {
						out = append(out, &types.User{ID: userID, Email: userID + "@example.test", Token: "secret-token", TokenType: types.TokenTypeAPIKey})
						break
					}
				}
			}
			return out, int64(len(out)), nil
		}).AnyTimes()

	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, q *store.GetUserQuery) (*types.User, error) {
			if _, ok := all[q.ID]; !ok {
				return nil, store.ErrNotFound
			}
			return &types.User{
				ID:           q.ID,
				Email:        q.ID + "@example.test",
				FullName:     "Synthetic " + q.ID,
				Token:        "secret-token",
				TokenType:    types.TokenTypeAPIKey,
				PasswordHash: []byte("hash"),
			}, nil
		}).AnyTimes()

	return &HelixAPIServer{Store: mockStore}
}

func callGetUserDetails(t *testing.T, srv *HelixAPIServer, caller *types.User, targetID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/"+targetID, nil)
	req = mux.SetURLVars(req, map[string]string{"id": targetID})
	req = req.WithContext(setTestRequestUser(req.Context(), caller))
	rr := httptest.NewRecorder()
	srv.getUserDetails(rr, req)
	return rr
}

func TestGetUserDetails_CrossTenant_TST01_145(t *testing.T) {
	tests := []struct {
		name       string
		caller     *types.User
		target     string
		wantStatus int
	}{
		// PoC: testorg2 key reads testorg synthetic user -> must now be 404.
		{"testorg2 admin key cannot read testorg user", &types.User{ID: tenantBAdmin, TokenType: types.TokenTypeAPIKey}, tenantAUser, http.StatusNotFound},
		// Reverse direction.
		{"testorg admin key cannot read testorg2 user", &types.User{ID: tenantAAdmin, TokenType: types.TokenTypeAPIKey}, tenantBUser, http.StatusNotFound},
		// Org-scoped key for the wrong org.
		{"org-scoped key for other org is denied", &types.User{ID: tenantBAdmin, OrganizationID: tenantBOrg, TokenType: types.TokenTypeAPIKey}, tenantAUser, http.StatusNotFound},
		// Org-scoped key claiming an org the caller isn't a member of.
		{"org-scoped key for non-member org is denied", &types.User{ID: tenantBAdmin, OrganizationID: tenantAOrg, TokenType: types.TokenTypeAPIKey}, tenantAUser, http.StatusNotFound},
		// Unknown ID looks identical to out-of-scope ID (no enumeration oracle).
		{"unknown user id is 404", &types.User{ID: tenantAAdmin}, "user-does-not-exist", http.StatusNotFound},
		// Owner controls still work.
		{"testorg admin reads testorg user", &types.User{ID: tenantAAdmin, TokenType: types.TokenTypeAPIKey}, tenantAUser, http.StatusOK},
		{"testorg2 admin reads testorg2 user", &types.User{ID: tenantBAdmin, TokenType: types.TokenTypeAPIKey}, tenantBUser, http.StatusOK},
		{"org-scoped key for shared org is allowed", &types.User{ID: tenantAAdmin, OrganizationID: tenantAOrg}, tenantAUser, http.StatusOK},
		{"user reads self", &types.User{ID: tenantBUser}, tenantBUser, http.StatusOK},
		{"platform admin reads any user", &types.User{ID: "platform-admin", Admin: true}, tenantAUser, http.StatusOK},
		// Org-scoped keys never escape their org, even for platform admins.
		{"admin's org-scoped key cannot read other org user", &types.User{ID: "platform-admin", Admin: true, OrganizationID: tenantBOrg}, tenantAUser, http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newUserDetailsTestServer(t)
			rr := callGetUserDetails(t, srv, tt.caller, tt.target)
			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())

			if tt.wantStatus == http.StatusNotFound {
				assert.NotContains(t, rr.Body.String(), "@example.test")
				assert.NotContains(t, rr.Body.String(), "Synthetic")
				return
			}

			var got map[string]interface{}
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			assert.Equal(t, tt.target, got["id"])
			assert.Empty(t, got["token"])
			assert.Empty(t, got["token_type"])
		})
	}
}

func callSearchUsers(t *testing.T, srv *HelixAPIServer, caller *types.User, orgID string) *httptest.ResponseRecorder {
	t.Helper()
	url := "/api/v1/users/search?query=example.test"
	if orgID != "" {
		url += "&organization_id=" + orgID
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req = req.WithContext(setTestRequestUser(req.Context(), caller))
	rr := httptest.NewRecorder()
	srv.searchUsers(rr, req)
	return rr
}

// The same disclosure via /users/search: without organization_id the search
// previously matched every user on the platform.
func TestSearchUsers_CrossTenant_TST01_145(t *testing.T) {
	tests := []struct {
		name       string
		caller     *types.User
		orgID      string
		wantStatus int
		wantUsers  []string
	}{
		{"non-admin without org is rejected", &types.User{ID: tenantBAdmin}, "", http.StatusBadRequest, nil},
		{"non-admin searching other tenant org is rejected", &types.User{ID: tenantBAdmin}, tenantAOrg, http.StatusForbidden, nil},
		{"org-scoped key searching other org is rejected", &types.User{ID: tenantBAdmin, OrganizationID: tenantBOrg}, tenantAOrg, http.StatusForbidden, nil},
		{"admin's org-scoped key without org is rejected", &types.User{ID: "platform-admin", Admin: true, OrganizationID: tenantBOrg}, "", http.StatusBadRequest, nil},
		{"unknown org is 404", &types.User{ID: tenantBAdmin}, "org_unknown", http.StatusNotFound, nil},
		{"member searches own org by id", &types.User{ID: tenantBAdmin}, tenantBOrg, http.StatusOK, []string{tenantBAdmin, tenantBUser}},
		{"member searches own org by name", &types.User{ID: tenantAAdmin}, "testorg", http.StatusOK, []string{tenantAAdmin, tenantAUser}},
		{"platform admin searches all users", &types.User{ID: "platform-admin", Admin: true}, "", http.StatusOK, []string{tenantAAdmin, tenantAUser, tenantBAdmin, tenantBUser}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newUserDetailsTestServer(t)
			rr := callSearchUsers(t, srv, tt.caller, tt.orgID)
			require.Equal(t, tt.wantStatus, rr.Code, rr.Body.String())
			if tt.wantStatus != http.StatusOK {
				assert.NotContains(t, rr.Body.String(), "@example.test")
				return
			}

			var got types.UserSearchResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &got))
			var ids []string
			for _, u := range got.Users {
				ids = append(ids, u.ID)
				assert.Empty(t, u.Token)
				assert.Empty(t, u.TokenType)
			}
			assert.ElementsMatch(t, tt.wantUsers, ids)
		})
	}
}
