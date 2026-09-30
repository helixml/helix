package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	"go.uber.org/mock/gomock"
)

func secretIntakeTestServer(t *testing.T) *HelixAPIServer {
	t.Helper()
	s, _ := newSecretIntakeTestServer(t)
	s.Cfg.ConnectPortal.SecretIntakeEnabled = true
	return s
}

func intakeOperatorRequest(method, path, body, projectID, intakeID string) *http.Request {
	r := intakeAuthenticatedRequest(method, path, body, projectID, "")
	return mux.SetURLVars(r, map[string]string{"id": projectID, "intake_id": intakeID})
}

func TestSecretIntakeBrowserAndConsumption(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust_1", ConversationID: "conv_1", Title: "Connect portal", Fields: []types.SecretIntakeField{{Name: "username", Label: "Username", Type: "text", Required: true}, {Name: "password", Label: "Password", Type: "password", Required: true}}}
	view, link, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
	require.NoError(t, err)
	require.Equal(t, "pending", view.Status)
	require.NotContains(t, link, "password")
	require.Contains(t, link, "/connect/intake/"+view.ID+"#")
	token := strings.Split(link, "#")[1]
	redeemInput, _ := json.Marshal(map[string]string{"intake_id": view.ID, "token": token})
	wrongID, _ := json.Marshal(map[string]string{"intake_id": "sci_wrong", "token": token})
	wrong := httptest.NewRecorder()
	s.redeemSecretIntake(wrong, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(wrongID)))
	require.Equal(t, http.StatusGone, wrong.Code)
	redeem := httptest.NewRecorder()
	s.redeemSecretIntake(redeem, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusNoContent, redeem.Code)
	cookies := redeem.Result().Cookies()
	require.Len(t, cookies, 2)
	require.True(t, cookies[0].HttpOnly)
	replay := httptest.NewRecorder()
	s.redeemSecretIntake(replay, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusGone, replay.Code)
	page := httptest.NewRecorder()
	s.serveSecretIntake(page, mux.SetURLVars(intakeRequest(http.MethodGet, "/connect/intake/"+view.ID, "", cookies...), map[string]string{"intake_id": view.ID}))
	require.Equal(t, http.StatusOK, page.Code)
	require.Contains(t, page.Body.String(), `name="password"`)
	require.Contains(t, page.Body.String(), `type="password"`)
	require.NotContains(t, page.Body.String(), token)
	require.NotContains(t, page.Body.String(), ">Continue</button>")
	router := mux.NewRouter()
	s.registerSecretIntakeRoutes(router, router.PathPrefix("/api/v1").Subrouter())
	routedPage := httptest.NewRecorder()
	router.ServeHTTP(routedPage, intakeRequest(http.MethodGet, "/connect/intake/"+view.ID, "", cookies...))
	require.Equal(t, http.StatusOK, routedPage.Code)
	require.Contains(t, routedPage.Body.String(), `name="password"`)
	routedBoot := httptest.NewRecorder()
	router.ServeHTTP(routedBoot, intakeRequest(http.MethodGet, "/connect/intake/boot.js", ""))
	require.Equal(t, http.StatusOK, routedBoot.Code)
	require.Contains(t, routedBoot.Header().Get("Content-Type"), "application/javascript")
	other := httptest.NewRecorder()
	s.serveSecretIntake(other, mux.SetURLVars(intakeRequest(http.MethodGet, "/connect/intake/sci_other", "", cookies...), map[string]string{"intake_id": "sci_other"}))
	require.NotContains(t, other.Body.String(), `name="password"`)
	boot := httptest.NewRecorder()
	s.secretIntakeBoot(boot, intakeRequest(http.MethodGet, "/connect/intake/boot.js", ""))
	require.Contains(t, boot.Body.String(), "fetch(\"/connect/intake/redeem\"")
	require.NotContains(t, boot.Body.String(), "addEventListener(\"click\"")
	bad := httptest.NewRecorder()
	s.submitSecretIntakeForm(bad, intakeRequest(http.MethodPost, "/connect/intake/submit", "csrf=bad&username=alice&password=secret", cookies...))
	require.Equal(t, http.StatusForbidden, bad.Code)
	form := url.Values{"csrf": {cookies[1].Value}, "username": {"alice"}, "password": {"secret-123"}}
	submit := httptest.NewRecorder()
	request := intakeRequest(http.MethodPost, "/connect/intake/submit", form.Encode(), cookies...)
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	s.submitSecretIntakeForm(submit, request)
	require.Equal(t, http.StatusSeeOther, submit.Code)
	require.Equal(t, "/connect/intake/"+view.ID, submit.Header().Get("Location"))
	require.NotContains(t, submit.Body.String(), "secret-123")
	status := httptest.NewRecorder()
	s.getSecretIntake(status, intakeOperatorRequest(http.MethodGet, "/api/v1/projects/prj_test/secret-intakes/"+view.ID, "", "prj_test", view.ID))
	require.Equal(t, http.StatusOK, status.Code)
	require.Contains(t, status.Body.String(), `"status":"submitted"`)
	require.NotContains(t, status.Body.String(), "secret-123")
	item, err := s.Store.GetSecretIntake(t.Context(), "prj_test", view.ID)
	require.NoError(t, err)
	require.NotContains(t, item.ValuesEncrypted, "secret-123")
	consumed := false
	require.NoError(t, s.ConsumeSecretIntake(t.Context(), "prj_test", view.ID, func(values map[string]string) error {
		consumed = true
		require.Equal(t, "secret-123", values["password"])
		return nil
	}))
	require.True(t, consumed)
	require.Error(t, s.ConsumeSecretIntake(t.Context(), "prj_test", view.ID, func(map[string]string) error { return nil }))
	newView, _, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
	require.NoError(t, err)
	newPath := "/api/v1/projects/prj_test/secret-intakes/" + newView.ID + "/submissions"
	newSubmission := httptest.NewRecorder()
	s.submitSecretIntakeAPI(newSubmission, intakeOperatorRequest(http.MethodPost, newPath, `{"values":{"username":"alice","password":"secret-456"}}`, "prj_test", newView.ID))
	require.Equal(t, http.StatusNoContent, newSubmission.Code)
	require.Error(t, s.ConsumeSecretIntake(t.Context(), "prj_test", newView.ID, func(map[string]string) error { return errors.New("connector failed") }))
	require.Error(t, s.ConsumeSecretIntake(t.Context(), "prj_test", newView.ID, func(map[string]string) error { return nil }))
	item, err = s.Store.GetSecretIntake(t.Context(), "prj_test", newView.ID)
	require.NoError(t, err)
	require.Empty(t, item.ValuesEncrypted)
}

func TestSecretIntakeConsumeEndpoint(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "Connect portal", Fields: []types.SecretIntakeField{{Name: "username", Label: "Username", Type: "text", Required: true}, {Name: "password", Label: "Password", Type: "password", Required: true}}}
	view, _, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
	require.NoError(t, err)
	path := "/api/v1/projects/prj_test/secret-intakes/" + view.ID + "/consume"
	missing := httptest.NewRecorder()
	s.consumeSecretIntakeAPI(missing, intakeOperatorRequest(http.MethodPost, path, "", "prj_test", "sci_missing"))
	require.Equal(t, http.StatusNotFound, missing.Code)
	pending := httptest.NewRecorder()
	s.consumeSecretIntakeAPI(pending, intakeOperatorRequest(http.MethodPost, path, "", "prj_test", view.ID))
	require.Equal(t, http.StatusConflict, pending.Code)
	cross := httptest.NewRecorder()
	s.consumeSecretIntakeAPI(cross, intakeOperatorRequest(http.MethodPost, path, "", "prj_other", view.ID))
	require.Equal(t, http.StatusNotFound, cross.Code)
	submitPath := "/api/v1/projects/prj_test/secret-intakes/" + view.ID + "/submissions"
	submit := httptest.NewRecorder()
	s.submitSecretIntakeAPI(submit, intakeOperatorRequest(http.MethodPost, submitPath, `{"values":{"username":"alice","password":"secret-789"}}`, "prj_test", view.ID))
	require.Equal(t, http.StatusNoContent, submit.Code)
	consume := httptest.NewRecorder()
	s.consumeSecretIntakeAPI(consume, intakeOperatorRequest(http.MethodPost, path, "", "prj_test", view.ID))
	require.Equal(t, http.StatusOK, consume.Code)
	require.Contains(t, consume.Body.String(), `"username":"alice"`)
	require.Contains(t, consume.Body.String(), `"password":"secret-789"`)
	repeat := httptest.NewRecorder()
	s.consumeSecretIntakeAPI(repeat, intakeOperatorRequest(http.MethodPost, path, "", "prj_test", view.ID))
	require.Equal(t, http.StatusConflict, repeat.Code)
	require.NotContains(t, repeat.Body.String(), "secret-789")
	item, err := s.Store.GetSecretIntake(t.Context(), "prj_test", view.ID)
	require.NoError(t, err)
	require.Equal(t, "consumed", item.Status)
	require.Empty(t, item.ValuesEncrypted)
}

func TestSecretIntakeWakeRouting(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockStore := store.NewMockStore(ctrl)
	s := &HelixAPIServer{Store: mockStore, Cfg: &config.ServerConfig{WebServer: config.WebServer{URL: "http://localhost:8080"}}}

	// No session bound (project-API create) — no store calls, no wake.
	s.notifySecretIntakeSubmitted(t.Context(), &types.SecretIntake{ID: "sci_no_session", ProjectID: "prj_test"})

	// Session bound but from another project — the wake must not fire.
	item := &types.SecretIntake{ID: "sci_cross", ProjectID: "prj_test", SessionID: "ses_other_project"}
	mockStore.EXPECT().GetSession(gomock.Any(), "ses_other_project").Return(&types.Session{ID: "ses_other_project", ProjectID: "prj_other"}, nil)
	s.notifySecretIntakeSubmitted(t.Context(), item)
}

func TestSecretIntakeDirectAPIAndValidation(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "API key", Fields: []types.SecretIntakeField{{Name: "api_key", Label: "API key", Type: "password", Required: true}}}
	view, _, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
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

func TestSecretIntakeLogoBranding(t *testing.T) {
	s := secretIntakeTestServer(t)
	base := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "Connect", Fields: []types.SecretIntakeField{{Name: "api_key", Label: "API key", Type: "password", Required: true}}}

	bad := base
	bad.LogoURL = "http://cdn.example.com/logo.png"
	_, _, err := s.createSecretIntake(t.Context(), "prj_test", bad, "")
	require.Error(t, err)

	relative := base
	relative.LogoURL = "/logo.png"
	_, _, err = s.createSecretIntake(t.Context(), "prj_test", relative, "")
	require.Error(t, err)

	ok := base
	ok.LogoURL = "https://cdn.example.com/logo.svg"
	view, link, err := s.createSecretIntake(t.Context(), "prj_test", ok, "")
	require.NoError(t, err)

	token := strings.Split(link, "#")[1]
	redeemInput, _ := json.Marshal(map[string]string{"intake_id": view.ID, "token": token})
	redeem := httptest.NewRecorder()
	s.redeemSecretIntake(redeem, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusNoContent, redeem.Code)
	page := httptest.NewRecorder()
	s.serveSecretIntake(page, mux.SetURLVars(intakeRequest(http.MethodGet, "/connect/intake/"+view.ID, "", redeem.Result().Cookies()...), map[string]string{"intake_id": view.ID}))
	require.Equal(t, http.StatusOK, page.Code)
	require.Contains(t, page.Body.String(), `src="https://cdn.example.com/logo.svg"`)
	require.NotContains(t, page.Body.String(), `class="mark"`)
}

func TestSecretIntakeURLOverrides(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "Connect", AccentColor: "#111111", Fields: []types.SecretIntakeField{{Name: "api_key", Label: "API key", Type: "password", Required: true}}}
	view, link, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
	require.NoError(t, err)
	token := strings.Split(link, "#")[1]
	redeemInput, _ := json.Marshal(map[string]string{"intake_id": view.ID, "token": token})
	redeem := httptest.NewRecorder()
	s.redeemSecretIntake(redeem, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusNoContent, redeem.Code)

	// Valid cosmetic overrides apply; an invalid color and a non-https logo are ignored.
	q := "?accent=%23ff8800&bg=%23101820&card=%23161f28&ink=%23eef3f5&brand=Acme&logo=http://evil/x.png"
	page := httptest.NewRecorder()
	s.serveSecretIntake(page, mux.SetURLVars(intakeRequest(http.MethodGet, "/connect/intake/"+view.ID+q, "", redeem.Result().Cookies()...), map[string]string{"intake_id": view.ID}))
	require.Equal(t, http.StatusOK, page.Code)
	body := page.Body.String()
	require.Contains(t, body, "--accent:#ff8800")
	require.Contains(t, body, "--bg:#101820")
	require.Contains(t, body, "--card:#161f28")
	require.Contains(t, body, "--ink:#eef3f5")
	require.Contains(t, body, ">Acme</p>")
	require.NotContains(t, body, "evil")
	// non-https logo override rejected → falls back to the embedded Helix logo
	require.NotContains(t, body, `class="mark"`)
	require.Contains(t, body, `src="data:image/png;base64,`)
}

func TestSecretIntakeDefaultLogo(t *testing.T) {
	s := secretIntakeTestServer(t)
	input := types.SecretIntakeCreateRequest{CustomerID: "cust", ConversationID: "conv", Title: "Connect", Fields: []types.SecretIntakeField{{Name: "api_key", Label: "API key", Type: "password", Required: true}}}
	view, link, err := s.createSecretIntake(t.Context(), "prj_test", input, "")
	require.NoError(t, err)
	token := strings.Split(link, "#")[1]
	redeemInput, _ := json.Marshal(map[string]string{"intake_id": view.ID, "token": token})
	redeem := httptest.NewRecorder()
	s.redeemSecretIntake(redeem, intakeRequest(http.MethodPost, "/connect/intake/redeem", string(redeemInput)))
	require.Equal(t, http.StatusNoContent, redeem.Code)
	page := httptest.NewRecorder()
	s.serveSecretIntake(page, mux.SetURLVars(intakeRequest(http.MethodGet, "/connect/intake/"+view.ID, "", redeem.Result().Cookies()...), map[string]string{"intake_id": view.ID}))
	require.Equal(t, http.StatusOK, page.Code)
	// No intake or override logo → the embedded Helix logo is the default header mark.
	require.Contains(t, page.Body.String(), `src="data:image/png;base64,`)
	require.NotContains(t, page.Body.String(), `class="mark"`)
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
	now := time.Now().UTC()
	item := types.SecretIntake{
		ID: "sci_expired", ProjectID: "prj_test", CustomerID: "cust", ConversationID: "chat",
		Title: "Expired", BrandName: "Helix Connect", AccentColor: "#00b8d4",
		Fields: []types.SecretIntakeField{{Name: "password", Label: "Password", Type: "password"}},
		Status: "submitted", ValuesEncrypted: "ciphertext", ValuesExpiresAt: now.Add(-time.Minute),
	}
	require.NoError(t, s.Store.CreateSecretIntake(t.Context(), &item))
	require.NoError(t, s.Store.ReapExpiredSecretIntakes(context.Background(), now))
	got, err := s.Store.GetSecretIntake(t.Context(), "prj_test", item.ID)
	require.NoError(t, err)
	require.Equal(t, "expired", got.Status)
	require.Empty(t, got.ValuesEncrypted)
}
