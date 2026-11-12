package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// newRegularAuthServer builds a server on the regular (password) provider
// with a session manager over the mock store. The mocks carry no
// expectations: any call not expected by a test fails it.
func newRegularAuthServer(t *testing.T) (*HelixAPIServer, *store.MockStore, *auth.MockAuthenticator) {
	t.Helper()
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	authn := auth.NewMockAuthenticator(ctrl)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderRegular
	cfg.Auth.RegistrationEnabled = true
	return &HelixAPIServer{
		Cfg:            cfg,
		Store:          st,
		authenticator:  authn,
		sessionManager: auth.NewSessionManager(st, nil, cfg),
	}, st, authn
}

// scopedKeyCallers are the credentials a sandbox, agent or org member is
// handed in place of the account's own: each grants less than the account.
var scopedKeyCallers = map[string]types.User{
	"session-scoped key": {ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, SessionID: "ses_1", ProjectID: "prj_1"},
	"project-scoped key": {ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, ProjectID: "prj_1"},
	"org-scoped key":     {ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI, OrganizationID: "org_1"},
	"embed key":          {ID: "usr_owner", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeEmbed, SpecTaskID: "spt_1"},
}

// A scoped key must not set its owner's password: that would turn a
// credential confined to one project or session into a full login as the
// owner. The authenticator mock fails the test on any UpdatePassword call.
func TestPasswordUpdate_RefusesScopedAPIKeys(t *testing.T) {
	for name, caller := range scopedKeyCallers {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newRegularAuthServer(t)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-update",
				jsonBody(t, types.PasswordUpdateRequest{NewPassword: "a-new-password"}))
			req = req.WithContext(setRequestUser(req.Context(), caller))
			rr := httptest.NewRecorder()

			s.passwordUpdate(rr, req)

			assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

// Changing the password from a browser session keeps that session and ends
// every other one.
func TestPasswordUpdate_SessionUserRevokesOtherSessions(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := types.User{ID: "usr_1", TokenType: types.TokenTypeSession}
	current := &types.UserSession{ID: "ses_current", UserID: user.ID, AuthProvider: types.AuthProviderRegular,
		ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now()}

	authn.EXPECT().UpdatePassword(gomock.Any(), user.ID, "a-new-password").Return(nil)
	st.EXPECT().GetUserSession(gomock.Any(), current.ID).Return(current, nil)
	st.EXPECT().GetUserSessionsByUser(gomock.Any(), user.ID).Return([]*types.UserSession{
		current,
		{ID: "ses_other_laptop", UserID: user.ID},
		{ID: "ses_stolen", UserID: user.ID},
	}, nil)
	st.EXPECT().DeleteUserSession(gomock.Any(), "ses_other_laptop").Return(nil)
	st.EXPECT().DeleteUserSession(gomock.Any(), "ses_stolen").Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-update",
		jsonBody(t, types.PasswordUpdateRequest{NewPassword: "a-new-password"}))
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: current.ID})
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()

	s.passwordUpdate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// A personal API key has no session to keep: every browser session ends.
func TestPasswordUpdate_PersonalKeyRevokesAllSessions(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := types.User{ID: "usr_1", TokenType: types.TokenTypeAPIKey, APIKeyType: types.APIkeytypeAPI}

	authn.EXPECT().UpdatePassword(gomock.Any(), user.ID, "a-new-password").Return(nil)
	st.EXPECT().DeleteUserSessionsByUser(gomock.Any(), user.ID).Return(nil)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-update",
		jsonBody(t, types.PasswordUpdateRequest{NewPassword: "a-new-password"}))
	req = req.WithContext(setRequestUser(req.Context(), user))
	rr := httptest.NewRecorder()

	s.passwordUpdate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
}

// The profile (name, commit identity, PR footer) is the account's, not the
// scoped key's. The store mock fails the test on any read or write.
func TestAccountUpdate_RefusesScopedAPIKeys(t *testing.T) {
	for name, caller := range scopedKeyCallers {
		t.Run(name, func(t *testing.T) {
			s, _, _ := newRegularAuthServer(t)
			commitEmail := "someone-else@example.com"
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/update",
				jsonBody(t, types.AccountUpdateRequest{GitCommitEmail: &commitEmail}))
			req = req.WithContext(setRequestUser(req.Context(), caller))
			rr := httptest.NewRecorder()

			s.accountUpdate(rr, req)

			assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
		})
	}
}

// Completing a reset ends every session the old password opened, then signs
// the browser in with a server-side session rather than a bare JWT cookie.
func TestPasswordResetComplete_RevokesSessionsAndStartsSession(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}

	authn.EXPECT().PasswordResetComplete(gomock.Any(), "reset-token", "a-new-password").Return(user, nil)
	gomock.InOrder(
		st.EXPECT().DeleteUserSessionsByUser(gomock.Any(), user.ID).Return(nil),
		st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
			require.Equal(t, user.ID, sess.UserID)
			require.Equal(t, types.AuthProviderRegular, sess.AuthProvider)
			return sess, nil
		}),
	)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset-complete",
		jsonBody(t, types.PasswordResetCompleteRequest{AccessToken: "reset-token", NewPassword: "a-new-password"}))
	rr := httptest.NewRecorder()

	s.passwordResetComplete(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertSessionCookiesOnly(t, rr)
}

// Sign-up signs the browser in the way a login does: a server-side session
// with its CSRF cookie, and no JWT cookie.
func TestRegister_StartsServerSession(t *testing.T) {
	s, _ := newRegisterTestServer(t)
	rr := register(t, s, `{"email":"invited@example.com","password":"pw","password_confirm":"pw"}`)
	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertSessionCookiesOnly(t, rr)
	var resp types.UserResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Empty(t, resp.Token, "no bearer token is handed to the browser")
}

func assertSessionCookiesOnly(t *testing.T, rr *httptest.ResponseRecorder) {
	t.Helper()
	names := map[string]bool{}
	for _, c := range rr.Result().Cookies() {
		names[c.Name] = c.Value != ""
	}
	assert.True(t, names[auth.SessionCookieName], "session cookie set: %v", names)
	assert.True(t, names[auth.CSRFCookieName], "CSRF cookie set: %v", names)
	assert.False(t, names[accessTokenCookie.Name], "no JWT access_token cookie: %v", names)
	assert.False(t, names[refreshTokenCookie.Name], "no JWT refresh_token cookie: %v", names)
}

// An admin reset locks out whoever held the old password: the user's
// sessions are revoked with it.
func TestAdminResetPassword_RevokesUserSessions(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	target := &types.User{ID: "usr_target", Email: "target@example.com"}

	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: target.ID}).Return(target, nil).Times(2)
	authn.EXPECT().UpdatePassword(gomock.Any(), target.ID, "a-new-password").Return(nil)
	st.EXPECT().DeleteUserSessionsByUser(gomock.Any(), target.ID).Return(nil)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/"+target.ID+"/password",
		jsonBody(t, types.AdminResetPasswordRequest{NewPassword: "a-new-password"}))
	req = mux.SetURLVars(req, map[string]string{"id": target.ID})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "usr_admin", Admin: true}))

	_, err := s.adminResetPassword(httptest.NewRecorder(), req)

	require.Nil(t, err)
}

// The legacy access_token cookie signs a request in by itself, exactly as the
// session cookie does, so a state-changing request carrying it needs the CSRF
// token too. Only header credentials skip the check.
func TestCSRFMiddleware_CookieCredentialsNeedToken(t *testing.T) {
	mw := newAuthMiddleware(nil, nil, nil, authMiddlewareConfig{}, nil, nil)
	handler := mw.csrfMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name    string
		method  string
		cookies []*http.Cookie
		csrf    string
		want    int
	}{
		{"access_token cookie, no CSRF header", http.MethodPost, []*http.Cookie{{Name: accessTokenCookie.Name, Value: "jwt"}}, "", http.StatusForbidden},
		{"access_token cookie with matching CSRF token", http.MethodPost, []*http.Cookie{{Name: accessTokenCookie.Name, Value: "jwt"}, {Name: auth.CSRFCookieName, Value: "csrf-1"}}, "csrf-1", http.StatusNoContent},
		{"session cookie, no CSRF header", http.MethodPut, []*http.Cookie{{Name: auth.SessionCookieName, Value: "ses"}}, "", http.StatusForbidden},
		{"session cookie with matching CSRF token", http.MethodDelete, []*http.Cookie{{Name: auth.SessionCookieName, Value: "ses"}, {Name: auth.CSRFCookieName, Value: "csrf-1"}}, "csrf-1", http.StatusNoContent},
		{"no cookies (header credential)", http.MethodPost, nil, "", http.StatusNoContent},
		{"access_token cookie on a GET", http.MethodGet, []*http.Cookie{{Name: accessTokenCookie.Name, Value: "jwt"}}, "", http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, "/api/v1/api_keys", nil)
			for _, c := range tc.cookies {
				req.AddCookie(c)
			}
			if tc.csrf != "" {
				req.Header.Set(auth.CSRFHeaderName, tc.csrf)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assert.Equal(t, tc.want, rr.Code, rr.Body.String())
		})
	}
}

// The login form answers an unknown address and a wrong password alike.
func TestLogin_UnknownEmailIsIndistinguishableFromWrongPassword(t *testing.T) {
	unknown := func(t *testing.T) *httptest.ResponseRecorder {
		s, st, authn := newRegularAuthServer(t)
		st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "nobody@example.com"}).Return(nil, store.ErrNotFound)
		// The same bcrypt work is spent as for a real account, against a
		// decoy no password matches.
		authn.EXPECT().ValidatePassword(gomock.Any(), gomock.Any(), "guess").DoAndReturn(func(_ any, u *types.User, _ string) error {
			require.Empty(t, u.ID)
			require.NotEmpty(t, u.PasswordHash)
			return assert.AnError
		})
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, types.LoginRequest{Email: "nobody@example.com", Password: "guess"}))
		rr := httptest.NewRecorder()
		s.login(rr, req)
		return rr
	}(t)
	wrongPassword := func(t *testing.T) *httptest.ResponseRecorder {
		s, st, authn := newRegularAuthServer(t)
		user := &types.User{ID: "usr_1", Email: "user@example.com"}
		st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: user.Email}).Return(user, nil)
		authn.EXPECT().ValidatePassword(gomock.Any(), user, "guess").Return(assert.AnError)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
			jsonBody(t, types.LoginRequest{Email: user.Email, Password: "guess"}))
		rr := httptest.NewRecorder()
		s.login(rr, req)
		return rr
	}(t)

	assert.Equal(t, http.StatusUnauthorized, unknown.Code)
	assert.Equal(t, wrongPassword.Code, unknown.Code)
	assert.Equal(t, wrongPassword.Body.String(), unknown.Body.String())
}

// A reset request for an unknown address is answered as for a known one. The
// authenticator mock fails the test if a reset is requested.
func TestPasswordReset_UnknownEmailAnswersLikeKnown(t *testing.T) {
	s, st, _ := newRegularAuthServer(t)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: "nobody@example.com"}).Return(nil, store.ErrNotFound)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset",
		jsonBody(t, types.PasswordResetRequest{Email: "nobody@example.com"}))
	rr := httptest.NewRecorder()

	s.passwordReset(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Body.String())
}

// Browser WebSockets carry the session cookie, so a page on another origin
// (including a same-site sandbox preview, which SameSite=Lax does not stop)
// must not be able to open one. Non-browser clients send no Origin.
func TestWebsocketOriginAllowed(t *testing.T) {
	cases := []struct {
		name      string
		origin    string
		host      string
		serverURL string
		want      bool
	}{
		{"no Origin header (non-browser client)", "", "app.example.com", "https://app.example.com", true},
		{"same host as request", "https://app.example.com", "app.example.com", "", true},
		{"same host, different case", "https://APP.example.com", "app.example.com", "", true},
		{"configured server URL behind a proxy", "https://app.example.com", "api-internal:8080", "https://app.example.com", true},
		{"same-site subdomain (sandbox preview)", "https://share-abc.dev.example.com", "app.example.com", "https://app.example.com", false},
		{"foreign origin", "https://evil.example.net", "app.example.com", "https://app.example.com", false},
		{"null origin", "null", "app.example.com", "https://app.example.com", false},
		{"different port", "http://app.example.com:3000", "app.example.com", "https://app.example.com", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/user", nil)
			req.Host = tc.host
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			assert.Equal(t, tc.want, websocketOriginAllowed(req, tc.serverURL))
		})
	}
}

// The user WebSocket refuses the handshake from a foreign origin even for a
// caller whose cookie would otherwise authorize the session stream.
func TestUserWebSocket_RefusesForeignOrigin(t *testing.T) {
	ctrl := gomock.NewController(t)
	st := store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderRegular
	user := &types.User{ID: "usr_1"}
	sessionCookie := &types.UserSession{ID: "ses_browser", UserID: user.ID, AuthProvider: types.AuthProviderRegular,
		ExpiresAt: time.Now().Add(time.Hour), LastUsedAt: time.Now()}
	st.EXPECT().GetUserSession(gomock.Any(), sessionCookie.ID).Return(sessionCookie, nil)
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{ID: user.ID}).Return(user, nil).AnyTimes()
	st.EXPECT().GetSession(gomock.Any(), "ses_chat").Return(&types.Session{ID: "ses_chat", Owner: user.ID}, nil)

	sm := auth.NewSessionManager(st, nil, cfg)
	s := &HelixAPIServer{
		Cfg:            cfg,
		Store:          st,
		sessionManager: sm,
		authMiddleware: newAuthMiddleware(nil, nil, st, authMiddlewareConfig{}, cfg, sm),
	}
	router := mux.NewRouter()
	s.startUserWebSocketServer(t.Context(), router, "/api/v1/ws/user")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ws/user?session_id=ses_chat", nil)
	req.Host = "localhost:8080"
	req.Header.Set("Origin", "https://share-abc.dev.localhost")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Sec-WebSocket-Version", "13")
	req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: sessionCookie.ID})
	rr := httptest.NewRecorder()

	router.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
}
