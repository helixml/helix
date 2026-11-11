package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// accountStates are the account states that must lose all API access.
var accountStates = map[string]func(*types.User){
	"waitlisted":  func(u *types.User) { u.Waitlisted = true },
	"deactivated": func(u *types.User) { u.Deactivated = true },
}

// credentialCase builds a request authenticated as user with one credential
// type, and the middleware that will see it.
type credentialCase struct {
	name string
	// via selects the entry point: extractMiddleware (the /api/v1 subrouter)
	// or auth() (the OpenAI/Anthropic-compatible /v1 routes).
	direct bool
	setup  func(t *testing.T, ctrl *gomock.Controller, st *store.MockStore, user *types.User) (*authMiddleware, *http.Request)
}

func apiKeyCase(name string, direct bool, method, path string, key types.ApiKey) credentialCase {
	return credentialCase{name: name, direct: direct, setup: func(_ *testing.T, _ *gomock.Controller, st *store.MockStore, user *types.User) (*authMiddleware, *http.Request) {
		key.Key = "hl-" + name
		key.Owner = user.ID
		key.OwnerType = types.OwnerTypeUser
		st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: key.Key}).Return(&key, nil)
		st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
		st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil).AnyTimes()
		r := httptest.NewRequest(method, path, nil)
		r.Header.Set("Authorization", "Bearer "+key.Key)
		return newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil), r
	}}
}

var credentialCases = []credentialCase{
	{name: "session cookie", setup: func(_ *testing.T, _ *gomock.Controller, st *store.MockStore, user *types.User) (*authMiddleware, *http.Request) {
		st.EXPECT().GetUserSession(gomock.Any(), "session-123").Return(&types.UserSession{
			ID: "session-123", UserID: user.ID, ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now(),
		}, nil)
		st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/provider-endpoints", nil)
		r.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: "session-123"})
		return newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, authpkg.NewSessionManager(st, nil, nil)), r
	}},
	{name: "helix user token", setup: func(_ *testing.T, ctrl *gomock.Controller, st *store.MockStore, user *types.User) (*authMiddleware, *http.Request) {
		a := authpkg.NewMockAuthenticator(ctrl)
		a.EXPECT().ValidateUserToken(gomock.Any(), "jwt-123").Return(user, nil)
		st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/provider-endpoints", nil)
		r.Header.Set("Authorization", "Bearer jwt-123")
		return newAuthMiddleware(a, nil, st, authMiddlewareConfig{}, nil, nil), r
	}},
	{name: "oidc access token", setup: func(_ *testing.T, ctrl *gomock.Controller, st *store.MockStore, user *types.User) (*authMiddleware, *http.Request) {
		o := authpkg.NewMockOIDC(ctrl)
		o.EXPECT().ValidateUserToken(gomock.Any(), "oidc-123").Return(user, nil)
		st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil)
		r := httptest.NewRequest(http.MethodPost, "/api/v1/provider-endpoints", nil)
		r.Header.Set("Authorization", "Bearer oidc-123")
		return newAuthMiddleware(nil, o, st, authMiddlewareConfig{}, nil, nil), r
	}},
	apiKeyCase("personal-key", false, http.MethodPost, "/api/v1/provider-endpoints", types.ApiKey{Type: types.APIkeytypeAPI}),
	apiKeyCase("org-key", false, http.MethodPost, "/api/v1/provider-endpoints", types.ApiKey{Type: types.APIkeytypeAPI, OrganizationID: "org-1"}),
	apiKeyCase("session-scoped-key", false, http.MethodGet, "/api/v1/sessions/ses-1", types.ApiKey{Type: types.APIkeytypeAPI, SessionID: "ses-1"}),
	apiKeyCase("embed-key", false, http.MethodGet, "/api/v1/auth/user", types.ApiKey{Type: types.APIkeytypeEmbed, SpecTaskID: "spt-1"}),
	apiKeyCase("openai-proxy-key", true, http.MethodPost, "/v1/chat/completions", types.ApiKey{Type: types.APIkeytypeAPI}),
	apiKeyCase("anthropic-proxy-key", true, http.MethodPost, "/v1/messages", types.ApiKey{Type: types.APIkeytypeAPI}),
}

// Every credential that acts as a user carries the user's account state: a
// waitlisted or deactivated account is refused with 403, and an approved one
// is served, through both the /api/v1 middleware and the /v1 proxy routes.
func TestAuthRejectsInactiveAccountsForEveryCredentialType(t *testing.T) {
	for _, cred := range credentialCases {
		states := map[string]func(*types.User){"active": func(*types.User) {}}
		for name, set := range accountStates {
			states[name] = set
		}
		for state, set := range states {
			t.Run(cred.name+"/"+state, func(t *testing.T) {
				ctrl := gomock.NewController(t)
				st := store.NewMockStore(ctrl)
				user := &types.User{ID: "user-123"}
				set(user)
				auth, request := cred.setup(t, ctrl, st, user)
				auth.lastSeenCache.Store(user.ID, time.Now())
				served := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
				response := httptest.NewRecorder()

				if cred.direct {
					auth.auth(served)(response, request)
				} else {
					auth.extractMiddleware(served).ServeHTTP(response, request)
				}

				if state == "active" {
					assert.Equal(t, http.StatusNoContent, response.Code)
					return
				}
				assert.Equal(t, http.StatusForbidden, response.Code)
				assert.Contains(t, response.Body.String(), user.AccountAccessError().Error())
			})
		}
	}
}

// Users are soft-deleted, so a deleted owner's row is not found. Their API
// keys must stop authenticating instead of acting as a bare user ID.
func TestGetUserFromTokenRejectsKeyWhoseOwnerIsGone(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hl-orphan"}).Return(&types.ApiKey{
		Key: "hl-orphan", Owner: "user-deleted", OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI,
	}, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: "user-deleted"}).Return(nil, store.ErrNotFound)
	auth := newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/provider-endpoints", nil)
	request.Header.Set("Authorization", "Bearer hl-orphan")
	response := httptest.NewRecorder()

	auth.extractMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)

	assert.Equal(t, http.StatusUnauthorized, response.Code)
}

// Runner and sandbox system tokens carry no account and must keep working.
func TestAuthServesRunnerToken(t *testing.T) {
	auth := newAuthMiddleware(nil, nil, nil, authMiddlewareConfig{runnerToken: "runner-secret"}, nil, nil)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/runners", nil)
	request.Header.Set("Authorization", "Bearer runner-secret")
	response := httptest.NewRecorder()

	auth.extractMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.True(t, isRunner(getRequestUser(r)))
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)

	assert.Equal(t, http.StatusNoContent, response.Code)
}

func TestUserWebSocketRejectsDeactivatedUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	mockAuth := authpkg.NewMockAuthenticator(ctrl)
	user := &types.User{ID: "user-123", Deactivated: true}
	mockAuth.EXPECT().ValidateUserToken(gomock.Any(), "token-123").Return(user, nil)
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil)
	server := &HelixAPIServer{Store: st, authMiddleware: newAuthMiddleware(mockAuth, nil, st, authMiddlewareConfig{}, nil, nil)}
	router := mux.NewRouter()
	server.startUserWebSocketServer(context.Background(), router, "/api/v1/ws/user")
	request := httptest.NewRequest(http.MethodGet, "/api/v1/ws/user?session_id=session-123", nil)
	request.Header.Set("Authorization", "Bearer token-123")
	response := httptest.NewRecorder()

	router.ServeHTTP(response, request)

	assert.Equal(t, http.StatusForbidden, response.Code)
}

// The GitHub-compatible API authenticates its own token (it is not behind
// extractMiddleware), so it must apply the same account-state rule.
func TestGitHubCompatRejectsInactiveAccount(t *testing.T) {
	for state, set := range accountStates {
		t.Run(state, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			user := &types.User{ID: "user-123"}
			set(user)
			st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: "hl-compat"}).Return(&types.ApiKey{
				Key: "hl-compat", Owner: user.ID, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI,
			}, nil)
			st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
			st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil)
			// No repository lookup: the request must stop at authentication.
			server := &HelixAPIServer{Store: st, authMiddleware: newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, nil, nil)}
			router := mux.NewRouter()
			server.registerGitHubCompatRoutes(router)
			request := httptest.NewRequest(http.MethodGet, githubCompatPrefix+"/repos/acme/widgets", nil)
			request.Header.Set("Authorization", "token hl-compat")
			response := httptest.NewRecorder()

			router.ServeHTTP(response, request)

			assert.Equal(t, http.StatusForbidden, response.Code)
			assert.Contains(t, response.Body.String(), user.AccountAccessError().Error())
		})
	}
}
