package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type portalTestStore struct {
	store.Store
	db *gorm.DB
}

func (s *portalTestStore) GormDB() *gorm.DB { return s.db }

func (s *portalTestStore) GetProject(_ context.Context, id string) (*types.Project, error) {
	return &types.Project{ID: id, UserID: "operator"}, nil
}

func portalTestServer(t *testing.T) (*HelixAPIServer, *gorm.DB) {
	t.Helper()
	t.Setenv("HELIX_ENCRYPTION_KEY", strings.Repeat("a", 64))
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.PortalConnectionAttempt{}))
	s := &HelixAPIServer{Store: &portalTestStore{db: db}, Cfg: &config.ServerConfig{}}
	s.Cfg.WebServer.URL = "http://localhost:8080"
	s.Cfg.PortalMockEnabled = true
	return s, db
}

func portalRequest(method, path, body string, cookies ...*http.Cookie) *http.Request {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if strings.HasSuffix(path, "/password") || strings.HasSuffix(path, "/otp") {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	return r
}

func portalOperatorRequest(method, path, body, projectID, connectionID string) *http.Request {
	r := portalRequest(method, path, body)
	r = r.WithContext(setRequestUser(r.Context(), types.User{ID: "operator"}))
	return mux.SetURLVars(r, map[string]string{"id": projectID, "connection_id": connectionID})
}

func TestPortalConnectionMockFlow(t *testing.T) {
	s, db := portalTestServer(t)
	token, err := portalRandomToken()
	require.NoError(t, err)
	now := time.Now().UTC()
	attempt := types.PortalConnectionAttempt{
		ID: "pca_test", ProjectID: "prj_test", CustomerID: "customer_1", ConversationID: "chat_1",
		Portal: "mock", BrandName: "Example Support", AccentColor: "#123456",
		Status: types.PortalConnectionPasswordPending, InvitationHash: portalHash(token),
		InvitationExpiresAt: now.Add(time.Minute),
	}
	require.NoError(t, db.Create(&attempt).Error)

	landing := httptest.NewRecorder()
	s.servePortalConnection(landing, portalRequest(http.MethodGet, "/connect", ""))
	require.Equal(t, http.StatusOK, landing.Code)
	require.Contains(t, landing.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'")
	require.NotContains(t, landing.Body.String(), token)

	input, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	redeem := httptest.NewRecorder()
	s.redeemPortalConnection(redeem, portalRequest(http.MethodPost, "/connect/redeem", string(input)))
	require.Equal(t, http.StatusNoContent, redeem.Code)
	require.Len(t, redeem.Result().Cookies(), 2)
	require.True(t, redeem.Result().Cookies()[0].HttpOnly)
	flowCookie, csrfCookie := redeem.Result().Cookies()[0], redeem.Result().Cookies()[1]

	replay := httptest.NewRecorder()
	s.redeemPortalConnection(replay, portalRequest(http.MethodPost, "/connect/redeem", string(input)))
	require.Equal(t, http.StatusGone, replay.Code)

	passwordPage := httptest.NewRecorder()
	s.servePortalConnection(passwordPage, portalRequest(http.MethodGet, "/connect", "", flowCookie, csrfCookie))
	require.Equal(t, http.StatusOK, passwordPage.Code)
	require.Contains(t, passwordPage.Body.String(), "Example Support")
	require.Contains(t, passwordPage.Body.String(), "background:#123456")
	require.Contains(t, passwordPage.Body.String(), csrfCookie.Value)

	passwordForm := url.Values{"username": {"demo"}, "password": {"demo-password"}, "csrf": {csrfCookie.Value}}
	wrongCSRF := httptest.NewRecorder()
	s.submitPortalPassword(wrongCSRF, portalRequest(http.MethodPost, "/connect/password", "username=demo&password=demo-password&csrf=wrong", flowCookie, csrfCookie))
	require.Equal(t, http.StatusForbidden, wrongCSRF.Code)
	password := httptest.NewRecorder()
	s.submitPortalPassword(password, portalRequest(http.MethodPost, "/connect/password", passwordForm.Encode(), flowCookie, csrfCookie))
	require.Equal(t, http.StatusSeeOther, password.Code)
	require.NotContains(t, password.Body.String(), "demo-password")

	otpForm := url.Values{"otp": {"123456"}, "csrf": {csrfCookie.Value}}
	otp := httptest.NewRecorder()
	s.submitPortalOTP(otp, portalRequest(http.MethodPost, "/connect/otp", otpForm.Encode(), flowCookie, csrfCookie))
	require.Equal(t, http.StatusSeeOther, otp.Code)

	require.NoError(t, db.First(&attempt, "id = ?", attempt.ID).Error)
	require.Equal(t, types.PortalConnectionConnected, attempt.Status)
	require.NotEmpty(t, attempt.SessionEncrypted)
	require.NotContains(t, attempt.SessionEncrypted, "demo-password")
	require.Equal(t, "", attempt.InvitationHash)

	operation := httptest.NewRecorder()
	s.mockPortalAccountStatus(operation, portalOperatorRequest(http.MethodGet, "/api/v1/projects/prj_test/portal-connections/pca_test/account-status", "", "prj_test", attempt.ID))
	require.Equal(t, http.StatusOK, operation.Code)
	require.Contains(t, operation.Body.String(), `"account_status":"active"`)
	require.NotContains(t, operation.Body.String(), attempt.SessionEncrypted)

	crossProject := httptest.NewRecorder()
	s.getPortalConnection(crossProject, portalOperatorRequest(http.MethodGet, "/api/v1/projects/prj_other/portal-connections/pca_test", "", "prj_other", attempt.ID))
	require.Equal(t, http.StatusNotFound, crossProject.Code)

	revoke := httptest.NewRecorder()
	s.revokePortalConnection(revoke, portalOperatorRequest(http.MethodDelete, "/api/v1/projects/prj_test/portal-connections/pca_test", "", "prj_test", attempt.ID))
	require.Equal(t, http.StatusNoContent, revoke.Code)
	operationAfterRevoke := httptest.NewRecorder()
	s.mockPortalAccountStatus(operationAfterRevoke, portalOperatorRequest(http.MethodGet, "/api/v1/projects/prj_test/portal-connections/pca_test/account-status", "", "prj_test", attempt.ID))
	require.Equal(t, http.StatusConflict, operationAfterRevoke.Code)
}

func TestCreatePortalConnectionUsesConfiguredOrigin(t *testing.T) {
	s, db := portalTestServer(t)
	request := portalOperatorRequest(http.MethodPost, "/api/v1/projects/prj_test/portal-connections", `{"customer_id":"customer_1","conversation_id":"chat_1","portal":"mock","brand_name":"Example Support","accent_color":"#123456"}`, "prj_test", "")
	request.Host = "attacker.example"
	response := httptest.NewRecorder()
	s.createPortalConnection(response, request)
	require.Equal(t, http.StatusCreated, response.Code)
	var result struct {
		InviteURL  string               `json:"invite_url"`
		Connection portalConnectionView `json:"connection"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &result))
	require.True(t, strings.HasPrefix(result.InviteURL, "http://localhost:8080/connect#"))
	require.NotContains(t, result.InviteURL, "attacker.example")
	require.Equal(t, types.PortalConnectionPasswordPending, result.Connection.Status)
	var attempt types.PortalConnectionAttempt
	require.NoError(t, db.First(&attempt, "id = ?", result.Connection.ID).Error)
	require.NotEmpty(t, attempt.InvitationHash)
	require.NotContains(t, response.Body.String(), "demo-password")

	s.Cfg.WebServer.URL = "http://public.example"
	insecure := httptest.NewRecorder()
	s.createPortalConnection(insecure, portalOperatorRequest(http.MethodPost, "/api/v1/projects/prj_test/portal-connections", `{"customer_id":"customer_1","conversation_id":"chat_1","portal":"mock"}`, "prj_test", ""))
	require.Equal(t, http.StatusInternalServerError, insecure.Code)
}

func TestPortalConnectionRoutes(t *testing.T) {
	s, _ := portalTestServer(t)
	router := mux.NewRouter()
	authRouter := router.PathPrefix("/api/v1").Subrouter()
	s.registerPortalConnectionRoutes(router, authRouter)
	for _, test := range []struct {
		method, path string
		want         int
	}{
		{http.MethodGet, "/connect", http.StatusOK},
		{http.MethodGet, "/connect/boot.js", http.StatusOK},
		{http.MethodPost, "/connect/redeem", http.StatusBadRequest},
		{http.MethodGet, "/connect/password", http.StatusMethodNotAllowed},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, portalRequest(test.method, test.path, ""))
		require.Equal(t, test.want, response.Code, test.path)
	}
}

func TestPortalConnectionHTTPFlow(t *testing.T) {
	s, db := portalTestServer(t)
	router := mux.NewRouter()
	s.registerPortalConnectionRoutes(router, router.PathPrefix("/api/v1").Subrouter())
	server := httptest.NewServer(router)
	defer server.Close()
	s.Cfg.WebServer.URL = server.URL
	token, err := portalRandomToken()
	require.NoError(t, err)
	require.NoError(t, db.Create(&types.PortalConnectionAttempt{
		ID: "pca_http", ProjectID: "prj_test", CustomerID: "customer_1", ConversationID: "chat_1",
		Portal: "mock", BrandName: "Example Support", AccentColor: "#123456",
		Status: types.PortalConnectionPasswordPending, InvitationHash: portalHash(token),
		InvitationExpiresAt: time.Now().UTC().Add(time.Minute),
	}).Error)
	jar, err := cookiejar.New(nil)
	require.NoError(t, err)
	client := &http.Client{Jar: jar}
	landing, err := client.Get(server.URL + "/connect#" + token)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, landing.StatusCode)
	require.NoError(t, landing.Body.Close())
	input, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	redeem, err := client.Post(server.URL+"/connect/redeem", "application/json", strings.NewReader(string(input)))
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, redeem.StatusCode)
	require.NoError(t, redeem.Body.Close())
	parsedURL, err := url.Parse(server.URL + "/connect")
	require.NoError(t, err)
	var csrf string
	for _, cookie := range jar.Cookies(parsedURL) {
		if cookie.Name == "helix_portal_csrf" {
			csrf = cookie.Value
		}
	}
	require.NotEmpty(t, csrf)
	password, err := client.PostForm(server.URL+"/connect/password", url.Values{"csrf": {csrf}, "username": {"demo"}, "password": {"demo-password"}})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, password.StatusCode)
	require.NoError(t, password.Body.Close())
	otp, err := client.PostForm(server.URL+"/connect/otp", url.Values{"csrf": {csrf}, "otp": {"123456"}})
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, otp.StatusCode)
	require.NoError(t, otp.Body.Close())
	var attempt types.PortalConnectionAttempt
	require.NoError(t, db.First(&attempt, "id = ?", "pca_http").Error)
	require.Equal(t, types.PortalConnectionConnected, attempt.Status)
}

func TestPortalConnectionRejectsMissingCSRFAndExpiredInvite(t *testing.T) {
	s, db := portalTestServer(t)
	token, err := portalRandomToken()
	require.NoError(t, err)
	attempt := types.PortalConnectionAttempt{
		ID: "pca_expired", ProjectID: "prj_test", CustomerID: "customer_1", ConversationID: "chat_1",
		Portal: "mock", BrandName: "Example", AccentColor: "#123456",
		Status: types.PortalConnectionPasswordPending, InvitationHash: portalHash(token),
		InvitationExpiresAt: time.Now().UTC().Add(-time.Minute),
	}
	require.NoError(t, db.Create(&attempt).Error)
	input, err := json.Marshal(map[string]string{"token": token})
	require.NoError(t, err)
	redeem := httptest.NewRecorder()
	s.redeemPortalConnection(redeem, portalRequest(http.MethodPost, "/connect/redeem", string(input)))
	require.Equal(t, http.StatusGone, redeem.Code)
}

func TestPortalConnectionLocksAfterBadPassword(t *testing.T) {
	s, db := portalTestServer(t)
	flow, err := portalRandomToken()
	require.NoError(t, err)
	csrf, err := portalRandomToken()
	require.NoError(t, err)
	attempt := types.PortalConnectionAttempt{
		ID: "pca_lockout", ProjectID: "prj_test", CustomerID: "customer_1", ConversationID: "chat_1",
		Portal: "mock", BrandName: "Example", AccentColor: "#123456",
		Status:   types.PortalConnectionPasswordPending,
		FlowHash: portalHash(flow), CSRFHash: portalHash(csrf),
		InvitationExpiresAt: time.Now().UTC().Add(time.Minute), FlowExpiresAt: time.Now().UTC().Add(time.Minute),
	}
	require.NoError(t, db.Create(&attempt).Error)
	form := url.Values{"csrf": {csrf}, "username": {"demo"}, "password": {"wrong"}}
	cookies := []*http.Cookie{{Name: portalFlowCookie, Value: flow}, {Name: "helix_portal_csrf", Value: csrf}}
	for i := 0; i < 3; i++ {
		response := httptest.NewRecorder()
		s.submitPortalPassword(response, portalRequest(http.MethodPost, "/connect/password", form.Encode(), cookies...))
		require.Equal(t, http.StatusSeeOther, response.Code)
	}
	require.NoError(t, db.First(&attempt, "id = ?", attempt.ID).Error)
	require.Equal(t, types.PortalConnectionFailed, attempt.Status)
	require.Equal(t, 3, attempt.PasswordAttempts)
	response := httptest.NewRecorder()
	s.submitPortalPassword(response, portalRequest(http.MethodPost, "/connect/password", form.Encode(), cookies...))
	require.Equal(t, http.StatusConflict, response.Code)
}
