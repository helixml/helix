package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The provider callback is a public GET. It must only complete the flow for
// the browser session that started it: the state proves a flow exists, not
// who is finishing it.
func newOAuthCallbackTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderRegular
	sessions := authpkg.NewSessionManager(st, nil, cfg)
	s := &HelixAPIServer{
		Cfg:            cfg,
		Store:          st,
		sessionManager: sessions,
		oauthManager:   oauth.NewManager(st, false),
	}
	s.authMiddleware = newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, cfg, sessions)
	return s, st
}

func oauthCallbackRequest(state, sessionID string) *http.Request {
	q := url.Values{"code": {"provider-code"}, "state": {state}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/oauth/flow/callback?"+q.Encode(), nil)
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: sessionID})
	}
	return req
}

func expectBrowserSession(st *store.MockStore, sessionID, userID string) {
	st.EXPECT().GetUserSession(gomock.Any(), sessionID).Return(&types.UserSession{
		ID: sessionID, UserID: userID, AuthProvider: types.AuthProviderRegular,
		ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now(),
	}, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: userID}).
		Return(&types.User{ID: userID, Email: userID + "@example.com"}, nil).AnyTimes()
	st.EXPECT().TouchUserLastSeen(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
}

func TestOAuthCallback_RefusesWithoutBrowserSession(t *testing.T) {
	s, st := newOAuthCallbackTestServer(t)
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "flow-state").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_starter", ProviderID: "prov-1", State: "flow-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)

	rr := httptest.NewRecorder()
	s.oauthCallbackHandler().ServeHTTP(rr, oauthCallbackRequest("flow-state", ""))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestOAuthCallback_RefusesAnotherUsersSession(t *testing.T) {
	s, st := newOAuthCallbackTestServer(t)
	expectBrowserSession(st, "sess-victim", "usr_victim")
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "attacker-state").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_attacker", ProviderID: "prov-1", State: "attacker-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)

	rr := httptest.NewRecorder()
	s.oauthCallbackHandler().ServeHTTP(rr, oauthCallbackRequest("attacker-state", "sess-victim"))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
	// The popup must learn it failed: the branded failure page posts
	// oauth-failure to the opener, never oauth-success.
	require.Equal(t, "text/html", rr.Header().Get("Content-Type"))
	require.Contains(t, rr.Body.String(), "oauth-failure")
	require.NotContains(t, rr.Body.String(), "oauth-success")
}

func TestOAuthCallback_CompletesForFlowStarter(t *testing.T) {
	s, st := newOAuthCallbackTestServer(t)
	expectBrowserSession(st, "sess-starter", "usr_starter")
	st.EXPECT().GetOAuthRequestTokenByState(gomock.Any(), "flow-state").Return([]*types.OAuthRequestToken{
		{ID: "req-1", UserID: "usr_starter", ProviderID: "prov-1", State: "flow-state", ExpiresAt: time.Now().Add(time.Minute)},
	}, nil)
	// Past the identity gate the manager resolves the provider; failing that
	// lookup is enough to prove the flow proceeded for the right user.
	st.EXPECT().GetOAuthProvider(gomock.Any(), "prov-1").Return(nil, errors.New("not found"))

	rr := httptest.NewRecorder()
	s.oauthCallbackHandler().ServeHTTP(rr, oauthCallbackRequest("flow-state", "sess-starter"))

	require.NotEqual(t, http.StatusForbidden, rr.Code, rr.Body.String())
	require.True(t, strings.Contains(rr.Body.String(), "oauth-failure"), rr.Body.String())
}
