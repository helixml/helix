package server

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	authpkg "github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// The Slack install callback is a public GET redirected from slack.com. It
// must only finish for the browser session of the org admin who started the
// install; the state proves which org and app, not who is completing it.
func newSlackCallbackTestServer(t *testing.T) (*HelixAPIServer, *store.MockStore, []byte) {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", "slack-oauth-test")
	key, err := crypto.GetEncryptionKey()
	require.NoError(t, err)
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.Auth.Provider = types.AuthProviderRegular
	sessions := authpkg.NewSessionManager(st, nil, cfg)
	s := &HelixAPIServer{Cfg: cfg, Store: st, sessionManager: sessions}
	s.authMiddleware = newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, cfg, sessions)
	return s, st, key
}

func slackCallbackRequest(state, sessionID string) *http.Request {
	q := url.Values{"code": {"slack-code"}, "state": {state}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/slack/oauth/callback?"+q.Encode(), nil)
	if sessionID != "" {
		req.AddCookie(&http.Cookie{Name: authpkg.SessionCookieName, Value: sessionID})
	}
	return req
}

func TestSlackOAuthCallback_RefusesWithoutBrowserSession(t *testing.T) {
	s, _, key := newSlackCallbackTestServer(t)
	state, err := encodeSlackOAuthState(key, "org-1", "app-1", "usr_admin", time.Now())
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	s.slackOAuthCallbackHandler().ServeHTTP(rr, slackCallbackRequest(state, ""))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestSlackOAuthCallback_RefusesAnotherUsersSession(t *testing.T) {
	s, st, key := newSlackCallbackTestServer(t)
	expectBrowserSession(st, "sess-victim", "usr_victim")
	state, err := encodeSlackOAuthState(key, "org-attacker", "app-1", "usr_attacker", time.Now())
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	s.slackOAuthCallbackHandler().ServeHTTP(rr, slackCallbackRequest(state, "sess-victim"))

	require.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}

func TestSlackOAuthCallback_ProceedsForInstallStarter(t *testing.T) {
	s, st, key := newSlackCallbackTestServer(t)
	expectBrowserSession(st, "sess-admin", "usr_admin")
	state, err := encodeSlackOAuthState(key, "org-1", "app-1", "usr_admin", time.Now())
	require.NoError(t, err)
	// Past the identity gate the handler resolves the global Slack app. An
	// unknown app redirects to the org settings page with an error, which is
	// enough to show the flow continued for the right user.
	st.EXPECT().GetServiceConnection(gomock.Any(), "app-1").Return(nil, store.ErrNotFound)

	rr := httptest.NewRecorder()
	s.slackOAuthCallbackHandler().ServeHTTP(rr, slackCallbackRequest(state, "sess-admin"))

	require.Equal(t, http.StatusFound, rr.Code, rr.Body.String())
	location, err := url.Parse(rr.Header().Get("Location"))
	require.NoError(t, err)
	require.Equal(t, "/orgs/org-1/general", location.Path)
}

// A state sealed before the user binding existed carries no starter and must
// not be accepted.
func TestSlackOAuthStateRequiresUser(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	if _, err := encodeSlackOAuthState(key, "org-1", "app-1", "", time.Now()); err == nil {
		t.Fatal("state without a user was sealed")
	}
}
