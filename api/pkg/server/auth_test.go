package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"golang.org/x/oauth2"
)

const (
	testAccessToken  = "test-access-token"
	testRefreshToken = "test-refresh-token"
	testState        = "test-state"
	testNonce        = "test-nonce"
	testCode         = "test-code"
	testEmail        = "test@example.com"
	testName         = "Test User"
	testServerURL    = "http://localhost:8080"
	newEmail         = "new@example.com"
	newName          = "New User"
)

type AuthSuite struct {
	suite.Suite
	ctrl          *gomock.Controller
	authCtx       context.Context
	server        *HelixAPIServer
	oidcClient    *auth.MockOIDC
	authenticator *auth.MockAuthenticator
	store         *store.MockStore
}

func TestAuthSuite(t *testing.T) {
	suite.Run(t, new(AuthSuite))
}

func (suite *AuthSuite) SetupTest() {
	ctrl := gomock.NewController(suite.T())
	suite.ctrl = ctrl
	suite.authCtx = context.Background()
	suite.store = store.NewMockStore(ctrl)
	cfg := &config.ServerConfig{}
	cfg.WebServer.URL = testServerURL
	cfg.Auth.Provider = types.AuthProviderOIDC
	suite.oidcClient = auth.NewMockOIDC(ctrl)
	suite.authenticator = auth.NewMockAuthenticator(ctrl)
	suite.server = &HelixAPIServer{
		Cfg:           cfg,
		oidcClient:    suite.oidcClient,
		authenticator: suite.authenticator,
		Store:         suite.store,
	}
}

// Helper functions
func (suite *AuthSuite) createTestRequest(method, path string, body []byte) *http.Request {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	return req.WithContext(suite.authCtx)
}

func (suite *AuthSuite) addCookie(req *http.Request, name, value string) {
	req.AddCookie(&http.Cookie{Name: name, Value: value})
}

func (suite *AuthSuite) createMockToken() *oauth2.Token {
	return &oauth2.Token{
		AccessToken:  testAccessToken,
		RefreshToken: testRefreshToken,
	}
}

func (suite *AuthSuite) createMockTokenNoRefresh() *oauth2.Token {
	return &oauth2.Token{
		AccessToken: testAccessToken,
	}
}

func (suite *AuthSuite) createMockTokenNoAccess() *oauth2.Token {
	return &oauth2.Token{
		RefreshToken: testRefreshToken,
	}
}

func (suite *AuthSuite) createMockUser() *types.User {
	return &types.User{
		ID:        "user123",
		Email:     testEmail,
		Username:  testEmail,
		FullName:  testName,
		CreatedAt: time.Now(),
	}
}

func (suite *AuthSuite) createMockUserInfo() *auth.UserInfo {
	return &auth.UserInfo{
		Subject:    "user123",
		Email:      testEmail,
		Name:       testName,
		GivenName:  "Test",
		FamilyName: "User",
	}
}

func (suite *AuthSuite) createMockUserUpdate() *auth.UserInfo {
	return &auth.UserInfo{
		Subject:    "user123",
		Email:      newEmail,
		Name:       newName,
		GivenName:  "Test",
		FamilyName: "User",
	}
}

// Test cases
func (suite *AuthSuite) TestLogin() {
	testCases := []struct {
		name           string
		method         string
		body           any
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name:           "OPTIONS request",
			method:         "OPTIONS",
			expectedStatus: http.StatusOK,
		},
		{
			name:   "Successful login",
			method: "POST",
			body: types.LoginRequest{
				RedirectURI: testServerURL + "/callback",
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().
					GetAuthURL(gomock.Any(), gomock.Any()).
					Return("http://mock-auth-url")
			},
			expectedStatus: http.StatusFound,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				res := rec.Result()
				defer res.Body.Close()
				suite.Equal("http://mock-auth-url", rec.Header().Get("Location"))
				cookies := res.Cookies()
				// Verify that the expected cookies are set
				for _, cookie := range cookies {
					suite.NotEmpty(cookie.Value)
					suite.Subset([]string{"state", "nonce", "redirect_uri"}, []string{cookie.Name})
					if cookie.Name == "redirect_uri" {
						suite.Equal(testServerURL+"/callback", cookie.Value)
					}
				}
			},
		},
		{
			name:           "Invalid JSON body",
			method:         "POST",
			body:           "invalid json",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "Invalid redirect URI",
			method: "POST",
			body: types.LoginRequest{
				RedirectURI: "http://malicious-site.com/callback",
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "empty web server url",
			method: "POST",
			body: types.LoginRequest{
				RedirectURI: testServerURL + "/callback",
			},
			setupMocks: func() {
				suite.server.Cfg.WebServer.URL = ""
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:   "empty redirect uri",
			method: "POST",
			setupMocks: func() {
				suite.oidcClient.EXPECT().
					GetAuthURL(gomock.Any(), gomock.Any()).
					Return("")
			},
			expectedStatus: http.StatusBadGateway,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			var reqBody []byte
			var err error

			switch v := tc.body.(type) {
			case string:
				reqBody = []byte(v)
			default:
				reqBody, err = json.Marshal(v)
				suite.NoError(err)
			}

			req := suite.createTestRequest(tc.method, "/api/v1/auth/login", reqBody)

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.login(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

func (suite *AuthSuite) TestCallback() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name: "Successful callback",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				suite.addCookie(req, "refresh_token", testRefreshToken)
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockToken()
				mockIDToken := &oidc.IDToken{Nonce: testNonce}
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(mockIDToken, nil)
			},
			expectedStatus: http.StatusFound,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				suite.Equal(testServerURL+"/dashboard", rec.Header().Get("Location"))
				// Verify access_token and refresh_token are set
				res := rec.Result()
				defer res.Body.Close()
				for _, cookie := range res.Cookies() {
					switch cookie.Name {
					case "access_token":
						suite.NotEmpty(cookie.Value)
						suite.Equal(testAccessToken, cookie.Value)
						suite.Equal("/", cookie.Path)
					case "refresh_token":
						suite.NotEmpty(cookie.Value)
						suite.Equal(testRefreshToken, cookie.Value)
						suite.Equal("/", cookie.Path)
					}
				}
			},
		},
		{
			name: "works without refresh_token", // Some IDPs don't return a refresh token
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockTokenNoRefresh()
				mockIDToken := &oidc.IDToken{Nonce: testNonce}
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(mockIDToken, nil)
			},
			expectedStatus: http.StatusFound,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				suite.Equal(testServerURL+"/dashboard", rec.Header().Get("Location"))
			},
		},
		{
			name: "missing access_token",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockTokenNoAccess()
				mockIDToken := &oidc.IDToken{Nonce: testNonce}
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(mockIDToken, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "Missing state cookie",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			expectedStatus: http.StatusFound, // Should redirect to the homepage
		},
		{
			name: "missing nonce cookie",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockToken()
				mockIDToken := &oidc.IDToken{Nonce: testNonce}
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(mockIDToken, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "test non matching state",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", "wrong-state", testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			expectedStatus: http.StatusFound,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				res := rec.Result()
				defer res.Body.Close()
				cookies := res.Cookies()
				for _, cookie := range cookies {
					suite.Equal(-1, cookie.MaxAge, "Cookie should be deleted")
					suite.Empty(cookie.Value, "Cookie value should be empty")
				}
				// Should redirect to the homepage
				suite.Equal(testServerURL, rec.Header().Get("Location"))
			},
		},
		{
			name: "empty code",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s", testState), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "exchange failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(nil, fmt.Errorf("exchange failure"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "verify id token failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockToken()
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(nil, fmt.Errorf("verify id token failure"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "nonce does not match id_token nonce",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", fmt.Sprintf("/api/v1/auth/callback?state=%s&code=%s", testState, testCode), nil)
				suite.addCookie(req, "state", testState)
				suite.addCookie(req, "nonce", testNonce)
				suite.addCookie(req, "redirect_uri", testServerURL+"/dashboard")
				return req
			},
			setupMocks: func() {
				mockToken := suite.createMockToken()
				mockIDToken := &oidc.IDToken{Nonce: "invalid-nonce"}
				suite.oidcClient.EXPECT().Exchange(gomock.Any(), testCode).Return(mockToken, nil)
				suite.oidcClient.EXPECT().VerifyIDToken(gomock.Any(), mockToken).Return(mockIDToken, nil)
			},
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			req := tc.setupRequest()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.callback(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

func (suite *AuthSuite) TestUser() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name: "options",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("OPTIONS", "/api/v1/auth/user", nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name: "Successful user info retrieval",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				user := suite.createMockUser()
				suite.oidcClient.EXPECT().ValidateUserToken(gomock.Any(), testAccessToken).Return(user, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				var response types.UserResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				suite.NoError(err)
				suite.Equal(testEmail, response.Email)
			},
		},
		{
			name: "missing access_token",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("GET", "/api/v1/auth/user", nil)
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name: "validate token failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().ValidateUserToken(gomock.Any(), testAccessToken).Return(nil, fmt.Errorf("validate token failure"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "validate token unauthorized",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().ValidateUserToken(gomock.Any(), testAccessToken).Return(nil, fmt.Errorf("401 unauthorized"))
			},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			req := tc.setupRequest()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.user(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

func (suite *AuthSuite) TestLogout() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name: "Successful logout",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("POST", "/api/v1/auth/logout", nil)
			},
			setupMocks: func() {
				logoutURL := "http://auth-provider/logout"
				suite.oidcClient.EXPECT().
					GetLogoutURL().
					Return(logoutURL, nil)
			},
			expectedStatus: http.StatusFound,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				res := rec.Result()
				defer res.Body.Close()
				suite.Equal("http://auth-provider/logout", rec.Header().Get("Location"))
				cookies := res.Cookies()
				for _, cookie := range cookies {
					suite.Equal(-1, cookie.MaxAge, "Cookie should be deleted")
					suite.Empty(cookie.Value, "Cookie value should be empty")
				}
				// Verify all required cookies are deleted
				cookieNames := map[string]bool{}
				for _, cookie := range cookies {
					cookieNames[cookie.Name] = true
				}
				requiredCookies := []string{"access_token", "refresh_token", "state", "nonce", "redirect_uri"}
				for _, name := range requiredCookies {
					suite.True(cookieNames[name], fmt.Sprintf("%s cookie should be deleted", name))
				}
			},
		},
		{
			name: "OPTIONS request",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("OPTIONS", "/api/v1/auth/logout", nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			req := tc.setupRequest()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.logout(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

func (suite *AuthSuite) TestAuthenticated() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name: "Valid access token",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/authenticated", nil)
				suite.addCookie(req, "access_token", "valid-token")
				return req
			},
			setupMocks: func() {
				suite.authenticator.EXPECT().
					ValidateUserToken(gomock.Any(), "valid-token").
					Return(&types.User{ID: "user123"}, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				var response types.AuthenticatedResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				suite.NoError(err)
				suite.True(response.Authenticated)
			},
		},
		{
			name: "Invalid access token",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/authenticated", nil)
				suite.addCookie(req, "access_token", "invalid-token")
				return req
			},
			setupMocks: func() {
				suite.authenticator.EXPECT().
					ValidateUserToken(gomock.Any(), "invalid-token").
					Return(nil, errors.New("invalid token"))
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				var response types.AuthenticatedResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				suite.NoError(err)
				suite.False(response.Authenticated)
			},
		},
		{
			name: "Missing access token",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("GET", "/api/v1/auth/authenticated", nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				var response types.AuthenticatedResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				suite.NoError(err)
				suite.False(response.Authenticated)
			},
		},
		{
			name: "OPTIONS request",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("OPTIONS", "/api/v1/auth/authenticated", nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			req := tc.setupRequest()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.authenticated(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

func (suite *AuthSuite) TestRefresh() {
	testCases := []struct {
		name           string
		setupRequest   func() *http.Request
		setupMocks     func()
		expectedStatus int
		checkResponse  func(*httptest.ResponseRecorder)
	}{
		{
			name: "Successful refresh",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("POST", "/api/v1/auth/refresh", nil)
				suite.addCookie(req, "refresh_token", testRefreshToken)
				return req
			},
			setupMocks: func() {
				newToken := &oauth2.Token{
					AccessToken:  "new-access-token",
					RefreshToken: "new-refresh-token",
				}
				suite.oidcClient.EXPECT().
					RefreshAccessToken(gomock.Any(), testRefreshToken).
					Return(newToken, nil)
			},
			expectedStatus: http.StatusNoContent,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				res := rec.Result()
				defer res.Body.Close()
				cookies := res.Cookies()
				var hasAccessToken, hasRefreshToken bool
				for _, cookie := range cookies {
					switch cookie.Name {
					case "access_token":
						hasAccessToken = true
						suite.Equal("new-access-token", cookie.Value)
					case "refresh_token":
						hasRefreshToken = true
						suite.Equal("new-refresh-token", cookie.Value)
					}
				}
				suite.True(hasAccessToken, "new access_token cookie should be set")
				suite.True(hasRefreshToken, "new refresh_token cookie should be set")
			},
		},
		{
			name: "Missing refresh token",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("POST", "/api/v1/auth/refresh", nil)
			},
			expectedStatus: http.StatusNoContent,
		},
		{
			name: "Refresh token failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("POST", "/api/v1/auth/refresh", nil)
				suite.addCookie(req, "refresh_token", "invalid-refresh-token")
				return req
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().
					RefreshAccessToken(gomock.Any(), "invalid-refresh-token").
					Return(nil, errors.New("refresh failed"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "OPTIONS request",
			setupRequest: func() *http.Request {
				return suite.createTestRequest("OPTIONS", "/api/v1/auth/refresh", nil)
			},
			expectedStatus: http.StatusOK,
		},
	}

	for _, tc := range testCases {
		suite.Run(tc.name, func() {
			rec := httptest.NewRecorder()
			req := tc.setupRequest()

			if tc.setupMocks != nil {
				tc.setupMocks()
			}

			suite.server.refresh(rec, req)
			suite.Equal(tc.expectedStatus, rec.Code)

			if tc.checkResponse != nil {
				tc.checkResponse(rec)
			}
		})
	}
}

// TestNewCookieManager_SecureCookiesAutoDetection tests that secure cookies
// are automatically enabled/disabled based on the SERVER_URL protocol.
// This ensures Safari compatibility (Safari rejects Secure cookies over HTTP)
// while maintaining security for HTTPS deployments.
func TestNewCookieManager_SecureCookiesAutoDetection(t *testing.T) {
	tests := []struct {
		name        string
		serverURL   string
		wantSecure  bool
		description string
	}{
		{
			name:        "HTTPS production URL enables secure cookies",
			serverURL:   "https://app.helix.ml",
			wantSecure:  true,
			description: "Production HTTPS deployments should use secure cookies",
		},
		{
			name:        "HTTPS with port enables secure cookies",
			serverURL:   "https://localhost:8443",
			wantSecure:  true,
			description: "HTTPS with explicit port should still use secure cookies",
		},
		{
			name:        "HTTPS subdomain enables secure cookies",
			serverURL:   "https://staging.helix.ml:443",
			wantSecure:  true,
			description: "HTTPS subdomains should use secure cookies",
		},
		{
			name:        "HTTP localhost disables secure cookies",
			serverURL:   "http://localhost:8080",
			wantSecure:  false,
			description: "Local dev HTTP should not use secure cookies (Safari compatibility)",
		},
		{
			name:        "HTTP test deployment disables secure cookies",
			serverURL:   "http://test.example.com:8080",
			wantSecure:  false,
			description: "HTTP test deployments should work without secure cookies",
		},
		{
			name:        "HTTP IP address disables secure cookies",
			serverURL:   "http://192.168.1.100:8080",
			wantSecure:  false,
			description: "HTTP with IP address should not use secure cookies",
		},
		{
			name:        "Empty URL defaults to non-secure",
			serverURL:   "",
			wantSecure:  false,
			description: "Empty URL should default to non-secure for safety",
		},
		{
			name:        "URL without scheme defaults to non-secure",
			serverURL:   "localhost:8080",
			wantSecure:  false,
			description: "URLs without scheme should default to non-secure",
		},
		{
			name:        "Uppercase HTTPS not matched (case sensitive)",
			serverURL:   "HTTPS://example.com",
			wantSecure:  false,
			description: "URL scheme matching is case-sensitive per RFC",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ServerConfig{}
			cfg.WebServer.URL = tt.serverURL

			cm := NewCookieManager(cfg)

			if cm.SecureCookies != tt.wantSecure {
				t.Errorf("NewCookieManager() SecureCookies = %v, want %v. %s",
					cm.SecureCookies, tt.wantSecure, tt.description)
			}
		})
	}
}

// TestNewCookieManager_ExplicitOverride tests that OIDC_SECURE_COOKIES=true
// forces secure cookies regardless of SERVER_URL protocol.
func TestNewCookieManager_ExplicitOverride(t *testing.T) {
	tests := []struct {
		name              string
		serverURL         string
		secureCookiesCfg  bool
		wantSecure        bool
		description       string
	}{
		{
			name:             "Override forces secure cookies on HTTP localhost",
			serverURL:        "http://localhost:8080",
			secureCookiesCfg: true,
			wantSecure:       true,
			description:      "OIDC_SECURE_COOKIES=true should force secure cookies even on HTTP",
		},
		{
			name:             "Override forces secure cookies on HTTP remote",
			serverURL:        "http://example.com",
			secureCookiesCfg: true,
			wantSecure:       true,
			description:      "OIDC_SECURE_COOKIES=true should force secure cookies for HTTPS proxy scenarios",
		},
		{
			name:             "No override uses auto-detect for HTTP",
			serverURL:        "http://localhost:8080",
			secureCookiesCfg: false,
			wantSecure:       false,
			description:      "OIDC_SECURE_COOKIES=false (default) should auto-detect from SERVER_URL",
		},
		{
			name:             "No override uses auto-detect for HTTPS",
			serverURL:        "https://app.helix.ml",
			secureCookiesCfg: false,
			wantSecure:       true,
			description:      "OIDC_SECURE_COOKIES=false with HTTPS URL should still use secure cookies",
		},
		{
			name:             "Override on HTTPS is redundant but works",
			serverURL:        "https://app.helix.ml",
			secureCookiesCfg: true,
			wantSecure:       true,
			description:      "OIDC_SECURE_COOKIES=true on HTTPS is redundant but should work",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.ServerConfig{}
			cfg.WebServer.URL = tt.serverURL
			cfg.Auth.OIDC.SecureCookies = tt.secureCookiesCfg

			cm := NewCookieManager(cfg)

			if cm.SecureCookies != tt.wantSecure {
				t.Errorf("NewCookieManager() SecureCookies = %v, want %v. %s",
					cm.SecureCookies, tt.wantSecure, tt.description)
			}
		})
	}
}
