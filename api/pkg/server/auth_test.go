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

	"github.com/coreos/go-oidc"
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
	cfg.Auth.Provider = types.AuthProviderKeycloak
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
				userInfo := suite.createMockUserInfo()
				user := suite.createMockUser()
				suite.oidcClient.EXPECT().GetUserInfo(gomock.Any(), testAccessToken).Return(userInfo, nil)
				suite.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(user, nil)
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
			name: "get userinfo failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				suite.oidcClient.EXPECT().GetUserInfo(gomock.Any(), testAccessToken).Return(nil, fmt.Errorf("get userinfo failure"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "get user failure",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				userInfo := suite.createMockUserInfo()
				suite.oidcClient.EXPECT().GetUserInfo(gomock.Any(), testAccessToken).Return(userInfo, nil)
				suite.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("get user failure"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "new user",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				userInfo := suite.createMockUserInfo()
				user := suite.createMockUser()
				suite.oidcClient.EXPECT().GetUserInfo(gomock.Any(), testAccessToken).Return(userInfo, nil)
				suite.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(nil, nil)
				suite.store.EXPECT().CreateUser(gomock.Any(), gomock.Any()).Return(user, nil)
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
			name: "updated user info",
			setupRequest: func() *http.Request {
				req := suite.createTestRequest("GET", "/api/v1/auth/user", nil)
				suite.addCookie(req, "access_token", testAccessToken)
				return req
			},
			setupMocks: func() {
				userInfo := suite.createMockUserUpdate()
				user := suite.createMockUser()
				suite.oidcClient.EXPECT().GetUserInfo(gomock.Any(), testAccessToken).Return(userInfo, nil)
				suite.store.EXPECT().GetUser(gomock.Any(), gomock.Any()).Return(user, nil)
				suite.store.EXPECT().UpdateUser(gomock.Any(), gomock.Any()).Return(user, nil)
			},
			expectedStatus: http.StatusOK,
			checkResponse: func(rec *httptest.ResponseRecorder) {
				var response types.UserResponse
				err := json.NewDecoder(rec.Body).Decode(&response)
				suite.NoError(err)
				suite.Equal(newEmail, response.Email)
				suite.Equal(newName, response.Name)
			},
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
