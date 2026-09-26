package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func secretIntakeTestServer(t *testing.T) *HelixAPIServer {
	t.Helper()
	s, db := portalTestServer(t)
	require.NoError(t, db.AutoMigrate(&types.SecretIntake{}))
	s.Cfg.SecretIntakeEnabled = true
	return s
}

func intakeOperatorRequest(method, path, body, projectID, intakeID string) *http.Request {
	r := portalOperatorRequest(method, path, body, projectID, "")
	return mux.SetURLVars(r, map[string]string{"id": projectID, "intake_id": intakeID})
}

func TestSecretIntakeBrowserAndConsumption(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust_1", ConversationID: "conv_1", Title: "Connect portal", Fields: []types.SecretIntakeField{{Name: "username", Label: "Username", Type: "text", Required: true}, {Name: "password", Label: "Password", Type: "password", Required: true}}}
	view, link, err := s.createSecretIntake(t.Context(), "prj_test", input)
	require.NoError(t, err)
	require.Equal(t, "pending", view.Status)
	require.NotContains(t, link, "password")
	token := strings.Split(link, "#")[1]
	redeemInput, _ := json.Marshal(map[string]string{"token": token})
	redeem := httptest.NewRecorder()
	s.redeemSecretIntake(redeem, portalRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusNoContent, redeem.Code)
	cookies := redeem.Result().Cookies()
	require.Len(t, cookies, 2)
	require.True(t, cookies[0].HttpOnly)
	replay := httptest.NewRecorder()
	s.redeemSecretIntake(replay, portalRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusGone, replay.Code)
	page := httptest.NewRecorder()
	s.serveSecretIntake(page, portalRequest(http.MethodGet, "/connect/intake", "", cookies...))
	require.Equal(t, http.StatusOK, page.Code)
	require.Contains(t, page.Body.String(), `name="password"`)
	require.Contains(t, page.Body.String(), `type="password"`)
	require.NotContains(t, page.Body.String(), token)
	bad := httptest.NewRecorder()
	s.submitSecretIntakeForm(bad, portalRequest(http.MethodPost, "/connect/intake/submit", "csrf=bad&username=alice&password=secret", cookies...))
	require.Equal(t, http.StatusForbidden, bad.Code)
	form := url.Values{"csrf": {cookies[1].Value}, "username": {"alice"}, "password": {"secret-123"}}
	submit := httptest.NewRecorder()
	request := portalRequest(http.MethodPost, "/connect/intake/submit", form.Encode(), cookies...)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.submitSecretIntakeForm(submit, request)
	require.Equal(t, http.StatusSeeOther, submit.Code)
	require.NotContains(t, submit.Body.String(), "secret-123")
	status := httptest.NewRecorder()
	s.getSecretIntake(status, intakeOperatorRequest(http.MethodGet, "/api/v1/projects/prj_test/secret-intakes/"+view.ID, "", "prj_test", view.ID))
	require.Equal(t, http.StatusOK, status.Code)
	require.Contains(t, status.Body.String(), `"status":"submitted"`)
	require.NotContains(t, status.Body.String(), "secret-123")
	var item types.SecretIntake
	require.NoError(t, s.Store.(interface{ GormDB() *gorm.DB }).GormDB().First(&item, "id = ?", view.ID).Error)
	require.NotContains(t, item.ValuesEncrypted, "secret-123")
	consumed := false
	require.NoError(t, s.ConsumeSecretIntake(t.Context(), "prj_test", view.ID, func(values map[string]string) error {
		consumed = true
		require.Equal(t, "secret-123", values["password"])
		return nil
	}))
	require.True(t, consumed)
	require.Error(t, s.ConsumeSecretIntake(t.Context(), "prj_test", view.ID, func(map[string]string) error { return nil }))
}

func TestSecretIntakeDirectAPIAndValidation(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "API key", Fields: []types.SecretIntakeField{{Name: "api_key", Label: "API key", Type: "password", Required: true}}}
	view, _, err := s.createSecretIntake(t.Context(), "prj_test", input)
	require.NoError(t, err)
	path := "/api/v1/projects/prj_test/secret-intakes/" + view.ID + "/submissions"
	wrong := httptest.NewRecorder()
	s.submitSecretIntakeAPI(wrong, intakeOperatorRequest(http.MethodPost, path, `{"values":{"api_key":"top-secret","extra":"x"}}`, "prj_test", view.ID))
	require.Equal(t, http.StatusBadRequest, wrong.Code)
	cross := httptest.NewRecorder()
	s.submitSecretIntakeAPI(cross, intakeOperatorRequest(http.MethodPost, path, `{"values":{"api_key":"top-secret"}}`, "prj_other", view.ID))
	require.Equal(t, http.StatusNotFound, cross.Code)
	good := httptest.NewRecorder()
	s.submitSecretIntakeAPI(good, intakeOperatorRequest(http.MethodPost, path, `{"values":{"api_key":"top-secret"}}`, "prj_test", view.ID))
	require.Equal(t, http.StatusNoContent, good.Code)
	require.NotContains(t, good.Body.String(), "top-secret")
}

func TestSecretIntakeArtifactSanitization(t *testing.T) {
	before, after, err := sanitizeSecretIntakeArtifact(`<html><body><h2 onclick="alert(1)">Connect</h2><script>fetch('https://bad.example')</script><div data-helix-form></div><p><img src="https://bad.example/x">Done</p></body></html>`)
	require.NoError(t, err)
	require.Equal(t, "<h2>Connect</h2>", before)
	require.Equal(t, "<p>Done</p>", after)
	_, _, err = sanitizeSecretIntakeArtifact(`<html><body><p>No form slot</p></body></html>`)
	require.Error(t, err)
}

func TestSecretIntakeReaperClearsExpiredCiphertext(t *testing.T) {
	s := secretIntakeTestServer(t)
	db := s.Store.(interface{ GormDB() *gorm.DB }).GormDB()
	now := time.Now().UTC()
	item := types.SecretIntake{
		ID: "sci_expired", ProjectID: "prj_test", CustomerID: "cust", ConversationID: "chat",
		Title: "Expired", BrandName: "Helix Connect", AccentColor: "#00b8d4",
		Fields: []types.SecretIntakeField{{Name: "password", Label: "Password", Type: "password"}},
		Status: "submitted", ValuesEncrypted: "ciphertext", ValuesExpiresAt: now.Add(-time.Minute),
	}
	require.NoError(t, db.Create(&item).Error)
	reapExpiredSecretIntakes(context.Background(), db, now)
	require.NoError(t, db.First(&item, "id = ?", item.ID).Error)
	require.Equal(t, "expired", item.Status)
	require.Empty(t, item.ValuesEncrypted)
}
