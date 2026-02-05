package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

type SessionManagerSuite struct {
	suite.Suite
	ctrl           *gomock.Controller
	mockStore      *store.MockStore
	mockOIDC       *MockOIDC
	sessionManager *SessionManager
	ctx            context.Context
	cfg            *config.ServerConfig
}

func TestSessionManagerSuite(t *testing.T) {
	suite.Run(t, new(SessionManagerSuite))
}

func (s *SessionManagerSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.ctx = context.Background()
	s.mockStore = store.NewMockStore(s.ctrl)
	s.mockOIDC = NewMockOIDC(s.ctrl)
	s.cfg = &config.ServerConfig{}
	s.cfg.WebServer.URL = "https://example.com"
	s.sessionManager = NewSessionManager(s.mockStore, s.mockOIDC, s.cfg)
}

func (s *SessionManagerSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *SessionManagerSuite) TestCreateSession() {
	userID := "usr_test123"
	authProvider := types.AuthProviderOIDC

	s.mockStore.EXPECT().
		CreateUserSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
			s.Equal(userID, session.UserID)
			s.Equal(authProvider, session.AuthProvider)
			s.NotEmpty(session.ID)
			s.True(session.ExpiresAt.After(time.Now()))
			return session, nil
		})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("User-Agent", "Test-Agent")

	session, err := s.sessionManager.CreateSession(
		s.ctx, w, r,
		userID,
		authProvider,
		"access_token",
		"refresh_token",
		time.Now().Add(time.Hour),
	)

	s.NoError(err)
	s.NotNil(session)
	s.Equal(userID, session.UserID)

	// Check cookies were set
	cookies := w.Result().Cookies()
	s.Require().Len(cookies, 2)

	var sessionCookie, csrfCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == SessionCookieName {
			sessionCookie = c
		}
		if c.Name == CSRFCookieName {
			csrfCookie = c
		}
	}

	s.NotNil(sessionCookie)
	s.True(sessionCookie.HttpOnly)
	s.True(sessionCookie.Secure)

	s.NotNil(csrfCookie)
	s.False(csrfCookie.HttpOnly) // JS needs to read this
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_NoSessionCookie() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.Nil(session)
	s.ErrorIs(err, ErrSessionNotFound)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_EmptySessionCookie() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: ""})

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.Nil(session)
	s.ErrorIs(err, ErrSessionNotFound)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_SessionNotFound() {
	sessionID := "uss_nonexistent"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(nil, gorm.ErrRecordNotFound)

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.Nil(session)
	s.ErrorIs(err, ErrSessionNotFound)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_SessionExpired() {
	sessionID := "uss_expired"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	expiredSession := &types.UserSession{
		ID:        sessionID,
		UserID:    "usr_test",
		ExpiresAt: time.Now().Add(-time.Hour), // Expired
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(expiredSession, nil)

	s.mockStore.EXPECT().
		DeleteUserSession(gomock.Any(), sessionID).
		Return(nil)

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.Nil(session)
	s.ErrorIs(err, ErrSessionExpired)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_ValidSession() {
	sessionID := "uss_valid"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	validSession := &types.UserSession{
		ID:         sessionID,
		UserID:     "usr_test",
		ExpiresAt:  time.Now().Add(time.Hour),
		LastUsedAt: time.Now(), // Recently used, no touch needed
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(validSession, nil)

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.NoError(err)
	s.NotNil(session)
	s.Equal(sessionID, session.ID)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_TouchesOldSession() {
	sessionID := "uss_old"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	oldSession := &types.UserSession{
		ID:         sessionID,
		UserID:     "usr_test",
		ExpiresAt:  time.Now().Add(time.Hour),
		LastUsedAt: time.Now().Add(-2 * time.Hour), // Last used more than 1 hour ago
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(oldSession, nil)

	s.mockStore.EXPECT().
		UpdateUserSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
			s.True(session.LastUsedAt.After(time.Now().Add(-time.Minute)))
			return session, nil
		})

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.NoError(err)
	s.NotNil(session)
}

func (s *SessionManagerSuite) TestDeleteSession() {
	sessionID := "uss_todelete"

	s.mockStore.EXPECT().
		DeleteUserSession(gomock.Any(), sessionID).
		Return(nil)

	w := httptest.NewRecorder()

	err := s.sessionManager.DeleteSession(s.ctx, w, sessionID)

	s.NoError(err)

	// Check cookies were cleared
	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == SessionCookieName || c.Name == CSRFCookieName {
			s.Equal(-1, c.MaxAge)
		}
	}
}

func (s *SessionManagerSuite) TestDeleteAllUserSessions() {
	userID := "usr_test123"

	s.mockStore.EXPECT().
		DeleteUserSessionsByUser(gomock.Any(), userID).
		Return(nil)

	w := httptest.NewRecorder()

	err := s.sessionManager.DeleteAllUserSessions(s.ctx, w, userID)

	s.NoError(err)
}

func (s *SessionManagerSuite) TestValidateCSRF_Valid() {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf-token"})
	r.Header.Set(CSRFHeaderName, "test-csrf-token")

	s.True(ValidateCSRF(r))
}

func (s *SessionManagerSuite) TestValidateCSRF_MissingCookie() {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.Header.Set(CSRFHeaderName, "test-csrf-token")

	s.False(ValidateCSRF(r))
}

func (s *SessionManagerSuite) TestValidateCSRF_MissingHeader() {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf-token"})

	s.False(ValidateCSRF(r))
}

func (s *SessionManagerSuite) TestValidateCSRF_Mismatch() {
	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "token-a"})
	r.Header.Set(CSRFHeaderName, "token-b")

	s.False(ValidateCSRF(r))
}

func (s *SessionManagerSuite) TestIsSecureCookies_HTTPS() {
	s.cfg.WebServer.URL = "https://example.com"
	s.True(s.sessionManager.isSecureCookies())
}

func (s *SessionManagerSuite) TestIsSecureCookies_HTTP() {
	s.cfg.WebServer.URL = "http://localhost:8080"
	s.cfg.Auth.OIDC.SecureCookies = false
	s.False(s.sessionManager.isSecureCookies())
}

func (s *SessionManagerSuite) TestIsSecureCookies_ExplicitTrue() {
	s.cfg.WebServer.URL = "http://localhost:8080"
	s.cfg.Auth.OIDC.SecureCookies = true
	s.True(s.sessionManager.isSecureCookies())
}

func (s *SessionManagerSuite) TestGetClientIP_XForwardedFor() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")

	s.Equal("192.168.1.1", getClientIP(r))
}

func (s *SessionManagerSuite) TestGetClientIP_XRealIP() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("X-Real-IP", "192.168.1.2")

	s.Equal("192.168.1.2", getClientIP(r))
}

func (s *SessionManagerSuite) TestGetClientIP_RemoteAddr() {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.168.1.3:12345"

	s.Equal("192.168.1.3:12345", getClientIP(r))
}

// Tests for OIDC token refresh scenarios (addresses PR review comment about OIDC_OFFLINE_ACCESS)

func (s *SessionManagerSuite) TestGetSessionFromRequest_OIDCRefreshNeeded_NoRefreshToken() {
	// Test case: OIDC session needs refresh but no refresh token available
	// This happens when OIDC_OFFLINE_ACCESS is not enabled
	sessionID := "uss_no_refresh_token"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	oidcSession := &types.UserSession{
		ID:               sessionID,
		UserID:           "usr_test",
		AuthProvider:     types.AuthProviderOIDC,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastUsedAt:       time.Now(),
		OIDCRefreshToken: "", // No refresh token (OIDC_OFFLINE_ACCESS not enabled)
		OIDCAccessToken:  "expired_access_token",
		OIDCTokenExpiry:  time.Now().Add(-time.Minute), // Token expired
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(oidcSession, nil)

	// Session should still be returned even without refresh capability
	// The access token is expired but we continue - API calls may fail and force re-login
	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.NoError(err)
	s.NotNil(session)
	s.Equal(sessionID, session.ID)
	s.Empty(session.OIDCRefreshToken) // No refresh token
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_OIDCRefreshNeeded_RefreshSucceeds() {
	// Test case: OIDC session needs refresh and refresh succeeds
	sessionID := "uss_refresh_success"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	oidcSession := &types.UserSession{
		ID:               sessionID,
		UserID:           "usr_test",
		AuthProvider:     types.AuthProviderOIDC,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastUsedAt:       time.Now(),
		OIDCRefreshToken: "valid_refresh_token",
		OIDCAccessToken:  "expired_access_token",
		OIDCTokenExpiry:  time.Now().Add(-time.Minute), // Token expired
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(oidcSession, nil)

	// Mock successful token refresh
	newExpiry := time.Now().Add(time.Hour)
	s.mockOIDC.EXPECT().
		RefreshAccessToken(gomock.Any(), "valid_refresh_token").
		Return(&oauth2.Token{
			AccessToken:  "new_access_token",
			RefreshToken: "new_refresh_token",
			Expiry:       newExpiry,
		}, nil)

	// Mock session update
	s.mockStore.EXPECT().
		UpdateUserSession(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, session *types.UserSession) (*types.UserSession, error) {
			s.Equal("new_access_token", session.OIDCAccessToken)
			s.Equal("new_refresh_token", session.OIDCRefreshToken)
			return session, nil
		})

	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.NoError(err)
	s.NotNil(session)
}

func (s *SessionManagerSuite) TestGetSessionFromRequest_OIDCRefreshNeeded_RefreshFails() {
	// Test case: OIDC session needs refresh but refresh fails
	// Session should still be returned - the expired token may still work or force re-login
	sessionID := "uss_refresh_fails"
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: SessionCookieName, Value: sessionID})

	oidcSession := &types.UserSession{
		ID:               sessionID,
		UserID:           "usr_test",
		AuthProvider:     types.AuthProviderOIDC,
		ExpiresAt:        time.Now().Add(time.Hour),
		LastUsedAt:       time.Now(),
		OIDCRefreshToken: "invalid_refresh_token",
		OIDCAccessToken:  "expired_access_token",
		OIDCTokenExpiry:  time.Now().Add(-time.Minute), // Token expired
	}

	s.mockStore.EXPECT().
		GetUserSession(gomock.Any(), sessionID).
		Return(oidcSession, nil)

	// Mock failed token refresh
	s.mockOIDC.EXPECT().
		RefreshAccessToken(gomock.Any(), "invalid_refresh_token").
		Return(nil, errors.New("refresh token expired"))

	// Session should still be returned despite refresh failure
	session, err := s.sessionManager.GetSessionFromRequest(s.ctx, r)

	s.NoError(err) // No error returned to caller
	s.NotNil(session)
	s.Equal(sessionID, session.ID)
}
