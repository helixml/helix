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

func TestUpdateGitRepository_OnlyOwnerCanRedirectUpstream(t *testing.T) {
	stored := &types.GitRepository{
		ID:             "repo_1",
		OwnerID:        "usr_alice",
		OrganizationID: "org_1",
		ExternalURL:    "https://github.com/acme/app.git",
		ExternalType:   types.ExternalRepositoryTypeGitHub,
		GitHub:         &types.GitHub{PersonalAccessToken: "alice-pat"},
	}

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{name: "external url", body: map[string]any{"external_url": "https://git.example.net/x.git", "external_type": "github"}},
		{name: "external type", body: map[string]any{"external_type": "gitlab"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			repo := *stored
			mockStore.EXPECT().GetGitRepository(gomock.Any(), "repo_1").Return(&repo, nil)
			mockStore.EXPECT().GetOrganizationMembership(gomock.Any(), gomock.Any()).
				Return(&types.OrganizationMembership{OrganizationID: "org_1", UserID: "usr_bob", Role: types.OrganizationRoleOwner}, nil).AnyTimes()
			mockStore.EXPECT().UpdateGitRepository(gomock.Any(), gomock.Any()).Times(0)

			server := &HelixAPIServer{
				Store:                mockStore,
				gitRepositoryService: services.NewGitRepositoryService(mockStore, t.TempDir(), "http://helix.test", "Helix", "helix@example.com"),
			}

			payload, err := json.Marshal(tc.body)
			require.NoError(t, err)
			req := httptest.NewRequest(http.MethodPut, "/api/v1/git/repositories/repo_1", bytes.NewReader(payload))
			req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "usr_bob"}))
			req = mux.SetURLVars(req, map[string]string{"id": "repo_1"})
			rr := httptest.NewRecorder()
			server.updateGitRepository(rr, req)

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}
