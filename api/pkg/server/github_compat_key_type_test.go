package server

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
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
