package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func authUserWithKey(t *testing.T, keyType types.APIKeyType) types.UserResponse {
	t.Helper()
	return authUserWithAPIKey(t, &types.ApiKey{Type: keyType})
}

// authUserWithAPIKey calls /auth/user with key. session, if given, is what the
// store returns for the key's session; otherwise the session is not found.
func authUserWithAPIKey(t *testing.T, key *types.ApiKey, session ...*types.Session) types.UserResponse {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	owner := &types.User{ID: "usr_owner", Email: "owner@example.com", FullName: "Owner Person", Admin: true}
	key.Key, key.Owner, key.OwnerType = "hl-key", owner.ID, types.OwnerTypeUser
	mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(key, nil)
	mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ any, _ *store.GetUserQuery) (*types.User, error) { u := *owner; return &u, nil }).AnyTimes()
	mockStore.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: owner.ID}, nil).AnyTimes()
	if len(session) > 0 {
		mockStore.EXPECT().GetSession(gomock.Any(), key.SessionID).Return(session[0], nil).AnyTimes()
	} else {
		mockStore.EXPECT().GetSession(gomock.Any(), gomock.Any()).Return(nil, store.ErrNotFound).AnyTimes()
	}

	server := &HelixAPIServer{
		Cfg:            &config.ServerConfig{},
		Store:          mockStore,
		authMiddleware: newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{}, nil, nil),
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/user", nil)
	req.Header.Set("Authorization", "Bearer hl-key")
	rr := httptest.NewRecorder()
	server.user(rr, req)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.UserResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	return resp
}

// Embed, bot instance and app keys are held by people other than the key's
// owner; /auth/user tells them the owner's ID and nothing of their profile.
func TestAuthUser_DelegatedKeysGetOnlyTheOwnerID(t *testing.T) {
	for _, keyType := range []types.APIKeyType{types.APIkeytypeEmbed, types.APIkeytypeBotInstance, types.APIkeytypeApp} {
		t.Run(string(keyType), func(t *testing.T) {
			resp := authUserWithKey(t, keyType)
			require.Equal(t, "usr_owner", resp.ID)
			require.Empty(t, resp.Email)
			require.Empty(t, resp.Name)
			require.False(t, resp.Admin)
		})
	}
}

func TestAuthUser_PersonalKeyGetsFullProfile(t *testing.T) {
	resp := authUserWithKey(t, types.APIkeytypeAPI)
	require.Equal(t, "owner@example.com", resp.Email)
}

// A desktop key outside a project (a sandbox's, or a session's with no
// project) is held by code in the container, so it gets the owner's ID only,
// like a delegated key. Project keys and a helix-org worker's session key
// keep the full profile.
func TestAuthUser_DesktopKeyGetsOnlyTheOwnerID(t *testing.T) {
	for _, c := range []struct {
		key     *types.ApiKey
		session *types.Session
	}{
		{&types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "sbx_A", OrganizationID: "org_1"}, nil},
		{&types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_A"}, &types.Session{ID: "ses_A"}},
	} {
		resp := authUserWithAPIKey(t, c.key, c.session)
		require.Equal(t, "usr_owner", resp.ID)
		require.Empty(t, resp.Email)
		require.Empty(t, resp.Name)
		require.False(t, resp.Admin)
	}

	worker := &types.Session{ID: "ses_W", Metadata: types.SessionMetadata{OrgWorkerID: "chief-of-staff"}}
	require.Equal(t, "owner@example.com", authUserWithAPIKey(t, &types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_W", OrganizationID: "org_1"}, worker).Email)
	require.Equal(t, "owner@example.com", authUserWithAPIKey(t, &types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "sbx_A", ProjectID: "prj_1"}).Email)
	require.Equal(t, "owner@example.com", authUserWithAPIKey(t, &types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses_A", ProjectID: "prj_1"}).Email)
}
