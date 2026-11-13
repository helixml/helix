package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// Production always wires authMiddleware, whose early token path also reads
// the access_token cookie. That path must not serve a legacy cookie before
// the cookie branch moves it onto a session, or the browser stays on the
// cookie and every write fails the CSRF check.

func newRegularServerWithMiddleware(t *testing.T) (*HelixAPIServer, *store.MockStore, *auth.MockAuthenticator) {
	t.Helper()
	s, st, authn := newRegularAuthServer(t)
	s.authMiddleware = newAuthMiddleware(authn, nil, st, authMiddlewareConfig{}, s.Cfg, s.sessionManager)
	return s, st, authn
}

func expectLegacyJWTUser(st *store.MockStore, authn *auth.MockAuthenticator, user *types.User) {
	authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(user, nil).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
}

func TestAuthenticated_WithMiddleware_MovesLegacyJWTCookieToSession(t *testing.T) {
	s, st, authn := newRegularServerWithMiddleware(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	expectLegacyJWTUser(st, authn, user)
	st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
		require.Equal(t, user.ID, sess.UserID)
		return sess, nil
	})

	rr := httptest.NewRecorder()
	s.authenticated(rr, legacyCookieRequest(http.MethodGet, "/api/v1/auth/authenticated"))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.AuthenticatedResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.True(t, resp.Authenticated)
	assertMovedToSession(t, rr)
}

func TestUser_WithMiddleware_MovesLegacyJWTCookieToSession(t *testing.T) {
	s, st, authn := newRegularServerWithMiddleware(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	expectLegacyJWTUser(st, authn, user)
	st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
		return sess, nil
	})

	rr := httptest.NewRecorder()
	s.user(rr, legacyCookieRequest(http.MethodGet, "/api/v1/auth/user"))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.UserResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, user.ID, resp.ID)
	assert.Empty(t, resp.Token)
	assertMovedToSession(t, rr)
}

// A credential the caller put in a header keeps taking the early path: no
// session is created for an API key (the store mock fails the test on
// CreateUserSession).
func TestAuthenticated_WithMiddleware_HeaderCredentialIsServedDirectly(t *testing.T) {
	s, st, _ := newRegularServerWithMiddleware(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	key := &types.ApiKey{Key: "hl-personal", Owner: user.ID, OwnerType: types.OwnerTypeUser, Type: types.APIkeytypeAPI}
	st.EXPECT().GetAPIKey(gomock.Any(), &types.ApiKey{Key: key.Key}).Return(key, nil).AnyTimes()
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil).AnyTimes()

	for _, path := range []string{"/api/v1/auth/authenticated", "/api/v1/auth/user"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+key.Key)
		rr := httptest.NewRecorder()
		if path == "/api/v1/auth/user" {
			s.user(rr, req)
		} else {
			s.authenticated(rr, req)
		}
		require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
		assert.Empty(t, rr.Result().Cookies(), "%s: no cookies for a header credential", path)
	}
}

// The OIDC branch behaves the same with the middleware wired: a legacy IdP
// token cookie is moved onto a session rather than served as-is.
func TestAuthenticated_WithMiddleware_MovesLegacyOIDCCookieToSession(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	oidcClient := auth.NewMockOIDC(ctrl)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	sm := auth.NewSessionManager(st, oidcClient, cfg)
	s := &HelixAPIServer{Cfg: cfg, Store: st, oidcClient: oidcClient, sessionManager: sm,
		authMiddleware: newAuthMiddleware(nil, oidcClient, st, authMiddlewareConfig{}, cfg, sm)}
	user := &types.User{ID: "idp-sub", Email: "user@example.com"}
	oidcClient.EXPECT().ValidateUserToken(gomock.Any(), "idp-access-token").Return(user, nil).AnyTimes()
	st.EXPECT().EnsureUserMeta(gomock.Any(), gomock.Any()).Return(&types.UserMeta{ID: user.ID}, nil).AnyTimes()
	st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
		require.Equal(t, types.AuthProviderOIDC, sess.AuthProvider)
		require.Equal(t, "idp-access-token", sess.OIDCAccessToken)
		return sess, nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/authenticated", nil)
	req.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: "idp-access-token"})
	req.AddCookie(&http.Cookie{Name: refreshTokenCookie.Name, Value: "idp-refresh-token"})
	rr := httptest.NewRecorder()
	s.authenticated(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertMovedToSession(t, rr)
}

// /auth/user reports an email collision as 409, as the callback does.
func TestUser_EmailCollisionIsConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	oidcClient := auth.NewMockOIDC(ctrl)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	s := &HelixAPIServer{Cfg: cfg, Store: st, oidcClient: oidcClient, sessionManager: auth.NewSessionManager(st, oidcClient, cfg)}
	oidcClient.EXPECT().ValidateUserToken(gomock.Any(), "idp-access-token").Return(nil, auth.ErrOIDCEmailCollision)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/user", nil)
	req.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: "idp-access-token"})
	rr := httptest.NewRecorder()
	s.user(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "already exists")
	assert.False(t, errors.Is(nil, auth.ErrOIDCEmailCollision))
}
