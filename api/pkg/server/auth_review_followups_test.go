package server

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// An SSO sign-in refused because its unverified email collides with an
// existing account is a 409 with the reason, not a 500.
func TestCallback_EmailCollisionIsConflict(t *testing.T) {
	ctrl := gomock.NewController(t)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	oidcClient := auth.NewMockOIDC(ctrl)
	st := store.NewMockStore(ctrl)
	s := &HelixAPIServer{Cfg: cfg, Store: st, oidcClient: oidcClient, sessionManager: auth.NewSessionManager(st, oidcClient, cfg)}

	token := (&oauth2.Token{AccessToken: "at", RefreshToken: "rt"}).WithExtra(map[string]any{"id_token": "idt"})
	oidcClient.EXPECT().Exchange(gomock.Any(), "code").Return(token, nil)
	oidcClient.EXPECT().VerifyIDToken(gomock.Any(), token).Return(&oidc.IDToken{Nonce: "nonce"}, nil)
	oidcClient.EXPECT().ValidateUserToken(gomock.Any(), "at").Return(nil, auth.ErrOIDCEmailCollision)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state=st&code=code", nil)
	req.AddCookie(&http.Cookie{Name: stateCookie.Name, Value: "st"})
	req.AddCookie(&http.Cookie{Name: nonceCookie.Name, Value: "nonce"})
	rr := httptest.NewRecorder()
	s.callback(rr, req)

	require.Equal(t, http.StatusConflict, rr.Code, rr.Body.String())
	assert.Contains(t, rr.Body.String(), "already exists")
}

// A reset request answers 200 whether or not the mail could be sent; the
// send happens after the response and a failure is only logged.
func TestPasswordReset_SendsAfterResponseAndHidesDeliveryFailure(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	st.EXPECT().GetUser(gomock.Any(), &store.GetUserQuery{Email: user.Email}).Return(user, nil)
	authn.EXPECT().RequestPasswordReset(gomock.Any(), user.Email).Return(errors.New("smtp down"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset",
		jsonBody(t, types.PasswordResetRequest{Email: user.Email}))
	rr := httptest.NewRecorder()
	s.passwordReset(rr, req)
	s.passwordResetSends.Wait()

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Empty(t, rr.Body.String())
}

// Completing a reset clears any legacy token cookie the browser still holds.
func TestPasswordResetComplete_ClearsLegacyCookies(t *testing.T) {
	s, st, authn := newRegularAuthServer(t)
	user := &types.User{ID: "usr_1", Email: "user@example.com"}
	authn.EXPECT().PasswordResetComplete(gomock.Any(), "reset-token", "a-new-password").Return(user, nil)
	st.EXPECT().DeleteUserSessionsByUser(gomock.Any(), user.ID).Return(nil)
	st.EXPECT().CreateUserSession(gomock.Any(), gomock.Any()).DoAndReturn(func(_ any, sess *types.UserSession) (*types.UserSession, error) {
		return sess, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset-complete",
		jsonBody(t, types.PasswordResetCompleteRequest{AccessToken: "reset-token", NewPassword: "a-new-password"}))
	req.AddCookie(&http.Cookie{Name: accessTokenCookie.Name, Value: "legacy-jwt"})
	rr := httptest.NewRecorder()
	s.passwordResetComplete(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())
	assertMovedToSession(t, rr)
}

// The desktop video and input proxies hijack the connection themselves, so
// the Origin policy is applied before the hijack: a foreign origin is a 403
// even for a caller authorized on the session.
func TestDesktopWebSocketProxies_RefuseForeignOrigin(t *testing.T) {
	for name, handler := range map[string]func(*HelixAPIServer, http.ResponseWriter, *http.Request){
		"stream": (*HelixAPIServer).proxyStreamWebSocket,
		"input":  (*HelixAPIServer).proxyInputWebSocket,
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			st := store.NewMockStore(ctrl)
			cfg := &config.ServerConfig{}
			cfg.WebServer.URL = testServerURL
			s := &HelixAPIServer{Cfg: cfg, Store: st}
			user := types.User{ID: "usr_1"}
			st.EXPECT().GetSession(gomock.Any(), "ses_1").Return(&types.Session{ID: "ses_1", Owner: user.ID}, nil).AnyTimes()

			req := httptest.NewRequest(http.MethodGet, "/api/v1/external-agents/ses_1/ws/"+name, nil)
			req.Host = "localhost:8080"
			req.Header.Set("Origin", "https://share-abc.dev.localhost")
			req = mux.SetURLVars(req, map[string]string{"sessionID": "ses_1"})
			req = req.WithContext(setRequestUser(req.Context(), user))
			rr := httptest.NewRecorder()

			handler(s, rr, req)

			assert.Equal(t, http.StatusForbidden, rr.Code, rr.Body.String())
			assert.Contains(t, rr.Body.String(), "origin")
		})
	}
}
