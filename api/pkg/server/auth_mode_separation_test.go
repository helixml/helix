package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/stripe"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// Password flows exist only for AUTH_PROVIDER=regular; OIDC flows only for
// AUTH_PROVIDER=oidc. Under OIDC the email-domain allow list and waitlist are
// enforced on SSO sign-in alone, so every password path must refuse before it
// reaches the store or the authenticator. The mocks below carry no
// expectations: any call into them fails the test.

func authModeConfig(provider types.AuthProvider) *config.ServerConfig {
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = provider
	cfg.Auth.RegistrationEnabled = true
	return cfg
}

func newAuthModeServer(t *testing.T, provider types.AuthProvider) *HelixAPIServer {
	ctrl := gomock.NewController(t)
	return &HelixAPIServer{
		Cfg:           authModeConfig(provider),
		Store:         store.NewMockStore(ctrl),
		authenticator: auth.NewMockAuthenticator(ctrl),
		oidcClient:    auth.NewMockOIDC(ctrl),
	}
}

func jsonBody(t *testing.T, v any) *bytes.Reader {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return bytes.NewReader(b)
}

func TestPasswordLogin_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login",
		jsonBody(t, types.LoginRequest{Email: "user@example.com", Password: "password123"}))
	rr := httptest.NewRecorder()

	s.login(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestPasswordReset_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset",
		jsonBody(t, types.PasswordResetRequest{Email: "user@example.com"}))
	rr := httptest.NewRecorder()

	s.passwordReset(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestPasswordResetComplete_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-reset-complete",
		jsonBody(t, types.PasswordResetCompleteRequest{AccessToken: "tok", NewPassword: "password123"}))
	rr := httptest.NewRecorder()

	s.passwordResetComplete(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestPasswordUpdate_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/password-update",
		jsonBody(t, types.PasswordUpdateRequest{NewPassword: "password123"}))
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "user-1"}))
	rr := httptest.NewRecorder()

	s.passwordUpdate(rr, req)

	assert.Equal(t, http.StatusForbidden, rr.Code)
}

func TestAdminResetPassword_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/admin/users/user-1/password",
		jsonBody(t, types.AdminResetPasswordRequest{NewPassword: "password123"}))
	req = mux.SetURLVars(req, map[string]string{"id": "user-1"})
	req = req.WithContext(setTestRequestUser(req.Context(), &types.User{ID: "admin-1", Admin: true}))

	_, err := s.adminResetPassword(httptest.NewRecorder(), req)

	require.Error(t, err)
	var httpErr *system.HTTPError
	require.ErrorAs(t, err, &httpErr)
	assert.Equal(t, http.StatusBadRequest, httpErr.StatusCode)
}

// The desktop shared secret signs in a regular-provider admin; under OIDC it
// would be a sign-in that never meets the identity provider.
func TestDesktopCallback_RefusedWhenAuthProviderIsOIDC(t *testing.T) {
	s := newAuthModeServer(t, types.AuthProviderOIDC)
	s.Cfg.Auth.DesktopAutoLoginSecret = "desktop-secret"
	req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/desktop-callback?token=desktop-secret", nil)
	rr := httptest.NewRecorder()

	s.desktopCallback(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code)
}

// Only AUTH_PROVIDER=oidc may start or finish an OIDC sign-in. An unknown
// provider value must not fall into the OIDC branch.
func TestOIDCLogin_RefusedWhenAuthProviderIsNotOIDC(t *testing.T) {
	for _, provider := range []types.AuthProvider{types.AuthProviderRegular, "keycloak"} {
		t.Run(string(provider), func(t *testing.T) {
			s := newAuthModeServer(t, provider)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", jsonBody(t, types.LoginRequest{}))
			rr := httptest.NewRecorder()

			s.login(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

func TestOIDCCallback_RefusedWhenAuthProviderIsNotOIDC(t *testing.T) {
	for _, provider := range []types.AuthProvider{types.AuthProviderRegular, "keycloak"} {
		t.Run(string(provider), func(t *testing.T) {
			s := newAuthModeServer(t, provider)
			req := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback?state=s&code=c", nil)
			req.AddCookie(&http.Cookie{Name: stateCookie.Name, Value: "s"})
			rr := httptest.NewRecorder()

			s.callback(rr, req)

			assert.Equal(t, http.StatusBadRequest, rr.Code)
		})
	}
}

// A password session left over from before a switch to OIDC (or minted by a
// password path) must not authenticate requests on an OIDC deployment.
func TestExtractMiddleware_RejectsRegularSessionWhenAuthProviderIsOIDC(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockStore := store.NewMockStore(ctrl)
	cfg := authModeConfig(types.AuthProviderOIDC)
	sessionManager := auth.NewSessionManager(mockStore, nil, cfg)
	mw := newAuthMiddleware(nil, nil, mockStore, authMiddlewareConfig{}, cfg, sessionManager)

	mockStore.EXPECT().GetUserSession(gomock.Any(), "uss_password").Return(&types.UserSession{
		ID:           "uss_password",
		UserID:       "usr_password",
		AuthProvider: types.AuthProviderRegular,
		ExpiresAt:    time.Now().Add(time.Hour),
		LastUsedAt:   time.Now().Add(time.Hour),
	}, nil)
	mockStore.EXPECT().DeleteUserSession(gomock.Any(), "uss_password").Return(nil)

	var seen *types.User
	handler := mw.extractMiddleware(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = getRequestUser(r)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: "uss_password"})
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.NotNil(t, seen)
	assert.Empty(t, seen.ID, "a regular session must not authenticate under OIDC")
	cleared := false
	for _, c := range rr.Result().Cookies() {
		if c.Name == auth.SessionCookieName && c.MaxAge < 0 {
			cleared = true
		}
	}
	assert.True(t, cleared, "the stale session cookie should be cleared")
}

func TestConfig_RegistrationNotAdvertisedWhenAuthProviderIsOIDC(t *testing.T) {
	for provider, want := range map[types.AuthProvider]bool{
		types.AuthProviderRegular: true,
		types.AuthProviderOIDC:    false,
	} {
		t.Run(string(provider), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mockStore := store.NewMockStore(ctrl)
			mockStore.EXPECT().GetSystemSettings(gomock.Any()).Return(&types.SystemSettings{}, nil)
			mockStore.EXPECT().ListProviderEndpoints(gomock.Any(), gomock.Any()).Return(nil, nil)
			s := &HelixAPIServer{
				Cfg:    authModeConfig(provider),
				Store:  mockStore,
				Stripe: stripe.NewStripe(config.Stripe{}, nil),
			}
			s.Cfg.FileStore.LocalFSPath = t.TempDir()

			cfg, err := s.getConfig(t.Context())

			require.NoError(t, err)
			assert.Equal(t, want, cfg.RegistrationEnabled)
		})
	}
}

func TestRegister_RefusedWhenAuthProviderIsUnknown(t *testing.T) {
	for _, provider := range []types.AuthProvider{"OIDC", "oidc ", "keycloak"} {
		cfg := &config.ServerConfig{}
		cfg.Auth.Provider = provider
		cfg.Auth.RegistrationEnabled = true
		s := &HelixAPIServer{Cfg: cfg}

		body := `{"email":"probe@example.com","password":"pw","password_confirm":"pw"}`
		req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		rr := httptest.NewRecorder()
		s.register(rr, req)

		require.Equal(t, http.StatusForbidden, rr.Code, string(provider))
	}
}

// An unrecognised provider must stop startup: otherwise OIDC is silently off
// while some password paths stay reachable.
func TestNewServer_RejectsUnknownAuthProvider(t *testing.T) {
	for _, provider := range []types.AuthProvider{"", "OIDC", "oidc ", "keycloak", "service"} {
		cfg := &config.ServerConfig{}
		cfg.WebServer.URL = "http://localhost:8080"
		cfg.WebServer.Host = "localhost"
		cfg.WebServer.Port = 8080
		cfg.WebServer.RunnerToken = "runner-token"
		cfg.Auth.Provider = provider

		s, err := NewServer(cfg, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
		require.Error(t, err, string(provider))
		require.Nil(t, s)
		require.Contains(t, err.Error(), "unsupported AUTH_PROVIDER")
	}
}
