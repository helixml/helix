package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

// The GitHub-compatible API authenticates its own Authorization header, so it
// must apply the key-type restrictions /api/v1 applies: a restricted key may
// not read or change repositories as its owner.
func TestGitHubCompat_RejectsRestrictedKeyTypes(t *testing.T) {
	for name, key := range map[string]*types.ApiKey{
		"embed":        {Key: "hl-embed", Owner: "usr_alice", OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeEmbed},
		"bot instance": {Key: "hl-bot", Owner: "usr_alice", OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeBotInstance},
		"sandbox":      {Key: "hl-sbx", Owner: "usr_alice", OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI, SessionID: "sbx_A", OrganizationID: "org_1"},
		"app":          {Key: "hl-app", Owner: "usr_alice", OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeApp, AppID: &sql.NullString{String: "app_1", Valid: true}},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetAPIKey(gomock.Any(), gomock.Any()).Return(key, nil)
			mockStore.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(&types.User{ID: "usr_alice"}, nil).AnyTimes()
			mockStore.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: "usr_alice"}, nil).AnyTimes()
			mockStore.EXPECT().GetGitRepository(gomock.Any(), gomock.Any()).Times(0)

			server := &HelixAPIServer{Store: mockStore, authMiddleware: &authMiddleware{store: mockStore}}
			router := mux.NewRouter()
			server.registerGitHubCompatRoutes(router)

			req := httptest.NewRequest(http.MethodGet, githubCompatPrefix+"/repos/alice/app/pulls", nil)
			req.Header.Set("Authorization", "token "+key.Key)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

// The runner token authenticates sandbox hosts, not a person: it may not act
// on repositories through the GitHub-compatible API, under either scheme.
func TestGitHubCompat_RejectsRunnerToken(t *testing.T) {
	cases := []struct{ method, path, auth string }{
		{http.MethodGet, "/repos/alice/app", "token runner-secret"},
		{http.MethodGet, "/repos/alice/app/pulls", "Bearer runner-secret"},
		{http.MethodPost, "/repos/alice/app/pulls", "token runner-secret"},
		{http.MethodPatch, "/repos/alice/app/pulls/1", "Bearer runner-secret"},
	}
	for _, tc := range cases {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl) // no store call is expected

			server := &HelixAPIServer{Store: mockStore, authMiddleware: newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{runnerToken: "runner-secret"}, nil, nil)}
			router := mux.NewRouter()
			server.registerGitHubCompatRoutes(router)

			req := httptest.NewRequest(tc.method, githubCompatPrefix+tc.path, strings.NewReader(`{}`))
			req.Header.Set("Authorization", tc.auth)
			rr := httptest.NewRecorder()
			router.ServeHTTP(rr, req)

			require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			require.Contains(t, rr.Body.String(), errRunnerTokenNotAccepted)
		})
	}
}
