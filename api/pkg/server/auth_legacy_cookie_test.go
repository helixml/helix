package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// legacyCookieRequest carries the JWT cookie that sign-up and reset used to
// set, with no session cookie.
func legacyCookieRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, path, nil)
	req.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: "legacy-jwt"})
	req.AddCookie(&http.Cookie{Name: refreshTokenCookie.Name, Value: "legacy-jwt"})
	return req
}

// assertMovedToSession checks that the response started a session and
// cleared the JWT cookies rather than setting new ones.
func assertMovedToSession(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	cookies := map[string]*http.Cookie{}
	for _, c := range rr.Result().Cookies() {
		cookies[c.Name] = c
	}
	require.Contains(t, cookies, auth.SessionCookieName)
	assert.NotEmpty(t, cookies[auth.SessionCookieName].Value)
	for _, name := range []string{accessTokenCookie.Name, refreshTokenCookie.Name} {
		require.Contains(t, cookies, name, "legacy cookie cleared")
		assert.Empty(t, cookies[name].Value, "%s is cleared, not renewed", name)
		assert.Less(t, cookies[name].MaxAge, 0, "%s is cleared, not renewed", name)
	}
}

// A browser still holding a valid legacy JWT cookie is moved onto a session
// by the authenticated check; the JWT is not renewed (the authenticator mock
// fails the test on GenerateUserToken).
func TestAuthenticated_MovesLegacyJWTCookieToSession(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(user, nil)
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

// An expired legacy JWT is not refreshed from the (identical) refresh cookie:
// the browser is simply not authenticated and signs in again.
func TestAuthenticated_ExpiredLegacyJWTIsNotRenewed(t *testing.T) {
	s, _, authn := newRegularAuthServer(t)
	authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(nil, errors.New("token validation failed: expired")).AnyTimes()

	rr := httptest.NewRecorder()
	s.authenticated(rr, legacyCookieRequest(http.MethodGet, "/api/v1/auth/authenticated"))

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	var resp types.AuthenticatedResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.False(t, resp.Authenticated)
	for _, c := range rr.Result().Cookies() {
		assert.Empty(t, c.Value, "no cookie is issued: %s", c.Name)
	}
}

// /auth/user does the same, and hands no bearer token to the browser.
func TestUser_MovesLegacyJWTCookieToSession(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(user, nil)
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

// /auth/refresh no longer mints a JWT for the regular provider: a valid one
// moves to a session, an invalid one is a 401.
func TestRefresh_RegularProviderDoesNotMintJWT(t *testing.T) {
	t.Run("valid legacy token", func(t *testing.T) {
		s, st, authn := newRegularAuthServer(t)
		user := &types.User{ID: "usr_1"}
		authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(user, nil)
		st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
			return sess, nil
		})

		rr := httptest.NewRecorder()
		s.refresh(rr, legacyCookieRequest(http.MethodPost, "/api/v1/auth/refresh"))

		require.Equal(t, http.StatusNoContent, rr.Code, rr.Body.String())
		assertMovedToSession(t, rr)
	})
	t.Run("expired legacy token", func(t *testing.T) {
		s, _, authn := newRegularAuthServer(t)
		authn.EXPECT().ValidateUserToken(gomock.Any(), "legacy-jwt").Return(nil, errors.New("expired"))

		rr := httptest.NewRecorder()
		s.refresh(rr, legacyCookieRequest(http.MethodPost, "/api/v1/auth/refresh"))

		require.Equal(t, http.StatusUnauthorized, rr.Code, rr.Body.String())
		assert.Empty(t, rr.Result().Cookies())
	})
}

// A failed code exchange is logged without the authorization code.
func TestCallback_DoesNotLogAuthorizationCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	oidcClient := auth.NewMockOIDC(ctrl)
	s := &HelixAPIServer{Cfg: cfg, Store: store.NewMockStore(ctrl), oidcClient: oidcClient}
	const code = "authorization-code-for-this-test-only"
	oidcClient.EXPECT().Exchange(gomock.Any(), code).Return(nil, errors.New("exchange failure"))

	var logs bytes.Buffer
	previous := log.Logger
	log.Logger = zerolog.New(&logs)
	t.Cleanup(func() { log.Logger = previous })

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state=st&code="+code, nil)
	req.AddCookie(&http.Cookie{Name: stateCookie.Name, Value: "st"})
	rr := httptest.NewRecorder()
	s.callback(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)
	assert.Contains(t, logs.String(), "Failed to exchange token")
	assert.NotContains(t, logs.String(), code)
}
