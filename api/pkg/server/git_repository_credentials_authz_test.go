package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// A repository's external URL is where its stored or pinned credentials are
// sent. These tests cover the ways a caller could aim someone else's
// credentials at a host of their choosing.

func postCreateRepository(t *testing.T, server *HelixAPIServer, user *types.User, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/git/repositories", bytes.NewReader(payload))
	req = req.WithContext(setTestRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()
	server.createGitRepository(rr, req)
	return rr
}

func TestCreateGitRepository_RejectsAnotherUsersOAuthConnection(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	mockStore.EXPECT().GetOAuthConnection(gomock.Any(), "conn_alice").
		Return(&types.OAuthConnection{ID: "conn_alice", UserID: "usr_alice"}, nil)

	server := &HelixAPIServer{Store: mockStore}
	rr := postCreateRepository(t, server, &types.User{ID: "usr_bob"}, map[string]any{
		"name":                "exfil",
		"owner_id":            "usr_bob",
		"is_external":         true,
		"external_url":        "https://git.example.net/x.git",
		"external_type":       "github",
		"oauth_connection_id": "conn_alice",
	})

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestCreateGitRepository_RejectsAnotherOwner(t *testing.T) {
	ctrl := gomock.NewController(t)
	server := &HelixAPIServer{Store: store.NewMockStore(ctrl)}

	rr := postCreateRepository(t, server, &types.User{ID: "usr_bob"}, map[string]any{
		"name":          "exfil",
		"owner_id":      "usr_alice",
		"is_external":   true,
		"external_url":  "https://git.example.net/x.git",
		"external_type": "github",
	})

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func gitRepositoryUpstreamUpdate(t *testing.T, user *types.User, role types.OrganizationRole, body map[string]any, expectUpdate bool) *httptest.ResponseRecorder {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	repo := types.GitRepository{
		ID:             "repo_1",
		OwnerID:        "usr_alice",
		OrganizationID: "org_1",
		ExternalURL:    "https://github.com/acme/app.git",
		ExternalType:   types.ExternalRepositoryTypeGitHub,
		GitHub:         &types.GitHub{PersonalAccessToken: "alice-pat"},
	}
	mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(&repo, nil).AnyTimes()
	mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
		Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: user.ID, Role: role}, nil).AnyTimes()
	// A plain member reaches update access through an access grant.
	mockStore.EXPECT().ListTeams(gomock.Any(), gomock.Any()).Return(nil, nil).AnyTimes()
	mockStore.EXPECT().ListAccessGrants(gomock.Any(), gomock.Any()).Return([]*types.AccessGrant{{
		Roles: []types.Role{{Config: types.Config{Rules: []types.Rule{{
			Resources: []types.Resource{types.ResourceGitRepository},
			Actions:   []types.Action{types.ActionGet, types.ActionUpdate},
			Effect:    types.EffectAllow,
		}}}}},
	}}, nil).AnyTimes()
	if expectUpdate {
		mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Return(nil).MinTimes(1)
	} else {
		mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Times(0)
	}

	server := &HelixAPIServer{
		Store:                mockStore,
		gitRepositoryService: services.NewGitRepositoryService(mockStore, t.TempDir(), "http://helix.test", "Helix", "helix@example.com"),
	}
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/git/repositories/repo_1", bytes.NewReader(payload))
	req = req.WithContext(setTestRequestUser(req.Context(), user))
	req = mux.SetURLVars(req, map[string]string{"id": "repo_1"})
	rr := httptest.NewRecorder()
	server.updateGitRepository(rr, req)
	return rr
}

func TestUpdateGitRepository_CollaboratorCannotRedirectUpstream(t *testing.T) {
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "external url", body: map[string]any{"external_url": "https://git.example.net/x.git", "external_type": "github"}},
		{name: "external type", body: map[string]any{"external_type": "gitlab"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rr := gitRepositoryUpstreamUpdate(t, &types.User{ID: "usr_bob"}, types.OrganizationRoleMember, tc.body, false)
			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

// The repository owner and an owner of its organization may still move it.
func TestUpdateGitRepository_OwnerAndOrgOwnerCanRedirectUpstream(t *testing.T) {
	body := map[string]any{"external_url": "https://github.com/acme/renamed.git"}
	for name, tc := range map[string]struct {
		user *types.User
		role types.OrganizationRole
	}{
		"repository owner": {&types.User{ID: "usr_alice"}, types.OrganizationRoleMember},
		"org owner":        {&types.User{ID: "usr_carol"}, types.OrganizationRoleOwner},
	} {
		t.Run(name, func(t *testing.T) {
			rr := gitRepositoryUpstreamUpdate(t, tc.user, tc.role, body, true)
			require.NotEqual(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}
