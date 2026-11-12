package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"gorm.io/gorm"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// endedIdPSession is an OIDC provider whose token endpoint rejects every
// refresh the way Keycloak does once the user's SSO session has ended.
func endedIdPSession(t *testing.T) (authpkg.OIDC, *atomic.Int32) {
	var server *httptest.Server
	var tokenRequests atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":                 server.URL,
			"authorization_endpoint": server.URL + "/auth",
			"token_endpoint":         server.URL + "/token",
			"jwks_uri":               server.URL + "/certs",
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, _ *http.Request) {
		tokenRequests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_grant", "error_description": "Session not active"})
	})
	server = httptest.NewServer(mux)
	t.Cleanup(server.Close)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	client, err := authpkg.NewOIDCClient(ctx, authpkg.OIDCConfig{
		ProviderURL:  server.URL,
		ClientID:     "helix",
		ClientSecret: "secret",
		RedirectURL:  testServerURL + "/callback",
	})
	require.NoError(t, err)
	return client, &tokenRequests
}

// endedSession is the stored row of a browser session whose identity-provider
// session has ended.
type endedSession struct {
	deleted atomic.Int32
	dead    atomic.Bool
}

func endedSessionStore(t *testing.T) (*store.MockStore, *endedSession) {
	st := store.NewMockStore(gomock.NewController(t))
	row := &endedSession{}
	st.EXPECT().GetUserSession(gomock.Any(), "uss_ended").DoAndReturn(func(context.Context, string) (*types.UserSession, error) {
		if row.deleted.Load() > 0 {
			return nil, gorm.ErrRecordNotFound
		}
		return &types.UserSession{
			ID:               "uss_ended",
			UserID:           "usr_ended",
			AuthProvider:     types.AuthProviderOIDC,
			ExpiresAt:        time.Now().Add(time.Hour),
			LastUsedAt:       time.Now(),
			OIDCAccessToken:  "expired",
			OIDCRefreshToken: "refresh",
			OIDCTokenExpiry:  time.Now().Add(-time.Minute),
			OIDCRefreshDead:  row.dead.Load(),
		}, nil
	}).AnyTimes()
	st.EXPECT().DeleteUserSessionWithRefreshToken(gomock.Any(), "uss_ended", "refresh").DoAndReturn(func(context.Context, string, string) (bool, error) {
		row.deleted.Add(1)
		return true, nil
	}).MaxTimes(1)
	st.EXPECT().MarkUserSessionOIDCRefreshDead(gomock.Any(), "uss_ended", "refresh").DoAndReturn(func(context.Context, string, string) (bool, error) {
		row.dead.Store(true)
		return true, nil
	}).MaxTimes(1)
	return st, row
}

func oidcServerConfig(endSession bool) *config.ServerConfig {
	cfg := &config.ServerConfig{}
	cfg.Auth.OIDC.EndSessionWhenIdPSessionEnds = endSession
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	return cfg
}

func assertSessionCookieCleared(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == authpkg.SessionCookieName {
			assert.Empty(t, c.Value)
			assert.Less(t, c.MaxAge, 0)
			return
		}
	}
	t.Fatalf("response did not clear the %s cookie", authpkg.SessionCookieName)
}

func endedSessionRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/apps", nil)
	req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: "uss_ended"})
	return req
}

// Default: the Helix session outlives the identity-provider session, as it
// always has, but asks the provider for a new token only once.
func TestExtractMiddleware_EndedIdPSessionKeepsSessionByDefault(t *testing.T) {
	oidcClient, tokenRequests := endedIdPSession(t)
	st, row := endedSessionStore(t)
	user := &types.User{ID: "usr_ended"}
	st.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(user, nil).AnyTimes()
	cfg := oidcServerConfig(false)
	sm := authpkg.NewSessionManager(st, oidcClient, cfg)
	auth := newAuthMiddleware(nil, oidcClient, st, authMiddlewareConfig{adminUserIDs: []string{user.ID}}, cfg, sm)
	auth.lastSeenCache.Store(user.ID, time.Now())
	handler := auth.extractMiddleware(requireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	for range 3 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, endedSessionRequest())
		require.Equal(t, http.StatusNoContent, rr.Code)
		assert.Empty(t, rr.Result().Cookies())
	}
	assert.True(t, row.dead.Load(), "the session no longer refreshes")
	assert.Equal(t, int32(0), row.deleted.Load())
	assert.LessOrEqual(t, tokenRequests.Load(), int32(2), "the provider is asked once (oauth2 may resend it once to detect the auth style)")
}

// OIDC_END_SESSION_WHEN_IDP_SESSION_ENDS: the browser gets a 401 and loses
// its session cookie, so the frontend sends it to sign in.
func TestExtractMiddleware_EndedIdPSessionIsUnauthorizedWhenConfigured(t *testing.T) {
	oidcClient, tokenRequests := endedIdPSession(t)
	st, row := endedSessionStore(t)
	cfg := oidcServerConfig(true)
	sm := authpkg.NewSessionManager(st, oidcClient, cfg)
	auth := newAuthMiddleware(nil, oidcClient, st, authMiddlewareConfig{}, cfg, sm)
	handler := auth.extractMiddleware(requireUser(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})))

	for i := range 3 {
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, endedSessionRequest())
		require.Equal(t, http.StatusUnauthorized, rr.Code)
		if i == 0 {
			assertSessionCookieCleared(t, rr)
		}
	}

	assert.Equal(t, int32(1), row.deleted.Load(), "the session is deleted")
	assert.LessOrEqual(t, tokenRequests.Load(), int32(2), "the provider is asked once (oauth2 may resend it once to detect the auth style)")
}

// /auth/authenticated is what the frontend checks to decide whether to show
// the login page; with the option on it reports false and clears the cookie.
func TestAuthenticated_EndedIdPSessionClearsCookie(t *testing.T) {
	oidcClient, _ := endedIdPSession(t)
	st, row := endedSessionStore(t)
	cfg := oidcServerConfig(true)
	s := &HelixAPIServer{Cfg: cfg, Store: st, oidcClient: oidcClient, sessionManager: authpkg.NewSessionManager(st, oidcClient, cfg)}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/authenticated", nil)
	req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: "uss_ended"})
	rr := httptest.NewRecorder()
	s.authenticated(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp types.AuthenticatedResponse
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.False(t, resp.Authenticated)
	assert.Equal(t, int32(1), row.deleted.Load())
	assertSessionCookieCleared(t, rr)
}
