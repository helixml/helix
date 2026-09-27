package server

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/mux"
	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

const secretIntakeCookie = "helix_secret_intake_flow"
const secretIntakeCSRFCookie = "helix_secret_intake_csrf"
const secretIntakeValuesTTL = time.Hour

var secretIntakeFieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var secretIntakeAutocomplete = map[string]bool{"off": true, "username": true, "current-password": true, "new-password": true, "one-time-code": true}

type secretIntakeInput = types.SecretIntakeCreateRequest

type SecretIntakeView struct {
	ID              string                    `json:"id"`
	ProjectID       string                    `json:"project_id"`
	CustomerID      string                    `json:"customer_id"`
	ConversationID  string                    `json:"conversation_id"`
	Status          string                    `json:"status"`
	Fields          []types.SecretIntakeField `json:"fields"`
	ExpiresAt       time.Time                 `json:"expires_at"`
	ValuesExpiresAt *time.Time                `json:"values_expires_at,omitempty"`
}

type SecretIntakeCreateResponse struct {
	Intake    SecretIntakeView `json:"intake"`
	InviteURL string           `json:"invite_url"`
}

type SecretIntakeSubmissionRequest struct {
	Values map[string]string `json:"values"`
}

func (s *HelixAPIServer) secretIntakeEnabled() bool {
	if !s.Cfg.ConnectPortal.SecretIntakeEnabled || os.Getenv("HELIX_ENCRYPTION_KEY") == "" || strings.TrimSpace(s.Cfg.WebServer.URL) == "" {
		return false
	}
	if _, err := s.getEncryptionKey(); err != nil {
		return false
	}
	return true
}

func secretIntakeStatus(item *types.SecretIntake) string {
	now := time.Now().UTC()
	if item.Status == "submitted" && !item.ValuesExpiresAt.After(now) {
		return "expired"
	}
	if item.Status == "pending" && ((!item.FlowExpiresAt.IsZero() && !item.FlowExpiresAt.After(now)) || (item.FlowExpiresAt.IsZero() && !item.InvitationExpiresAt.After(now))) {
		return "expired"
	}
	return item.Status
}

func secretIntakeResponse(item *types.SecretIntake) SecretIntakeView {
	view := SecretIntakeView{ID: item.ID, ProjectID: item.ProjectID, CustomerID: item.CustomerID, ConversationID: item.ConversationID, Status: secretIntakeStatus(item), Fields: item.Fields, ExpiresAt: item.InvitationExpiresAt}
	if !item.ValuesExpiresAt.IsZero() {
		view.ValuesExpiresAt = &item.ValuesExpiresAt
	}
	return view
}

func validateSecretIntakeInput(input *secretIntakeInput) error {
	input.CustomerID = strings.TrimSpace(input.CustomerID)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.BrandName = strings.TrimSpace(input.BrandName)
	if input.CustomerID == "" || len(input.CustomerID) > 128 || input.ConversationID == "" || len(input.ConversationID) > 128 {
		return errors.New("customer_id and conversation_id are required (max 128 characters)")
	}
	if input.Title == "" || len(input.Title) > 100 || len(input.Description) > 300 {
		return errors.New("invalid title or description")
	}
	if input.BrandName == "" {
		input.BrandName = "Helix Connect"
	}
	if len(input.BrandName) > 80 {
		return errors.New("brand_name is too long")
	}
	if input.AccentColor == "" {
		input.AccentColor = "#00b8d4"
	}
	if !portalAccentPattern.MatchString(input.AccentColor) {
		return errors.New("accent_color must be a six-digit hex color")
	}
	if len(input.Fields) == 0 || len(input.Fields) > 8 {
		return errors.New("fields must contain 1 to 8 entries")
	}
	seen := map[string]bool{}
	for i := range input.Fields {
		field := &input.Fields[i]
		field.Name = strings.TrimSpace(field.Name)
		field.Label = strings.TrimSpace(field.Label)
		if !secretIntakeFieldName.MatchString(field.Name) || field.Name == "csrf" || seen[field.Name] {
			return errors.New("invalid or duplicate field name")
		}
		seen[field.Name] = true
		if field.Label == "" || len(field.Label) > 80 {
			return errors.New("invalid field label")
		}
		if field.Type != "text" && field.Type != "password" {
			return errors.New("field type must be text or password")
		}
		if field.Autocomplete == "" {
			field.Autocomplete = "off"
		}
		if !secretIntakeAutocomplete[field.Autocomplete] {
			return errors.New("invalid autocomplete value")
		}
	}
	return nil
}

func (s *HelixAPIServer) createSecretIntake(ctx context.Context, projectID string, input secretIntakeInput) (SecretIntakeView, string, error) {
	if !s.secretIntakeEnabled() {
		return SecretIntakeView{}, "", errors.New("secret intake is disabled")
	}
	if err := validateSecretIntakeInput(&input); err != nil {
		return SecretIntakeView{}, "", err
	}
	baseURL, err := url.Parse(s.Cfg.WebServer.URL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "https" && !(baseURL.Scheme == "http" && (baseURL.Hostname() == "localhost" || baseURL.Hostname() == "127.0.0.1" || baseURL.Hostname() == "::1"))) {
		return SecretIntakeView{}, "", errors.New("server URL must use HTTPS")
	}
	var artifactBefore, artifactAfter string
	if input.ArtifactID != "" {
		artifactBefore, artifactAfter, err = s.loadSecretIntakeArtifact(ctx, projectID, input.ArtifactID)
		if err != nil {
			return SecretIntakeView{}, "", err
		}
	}
	token, err := portalRandomToken()
	if err != nil {
		return SecretIntakeView{}, "", err
	}
	now := time.Now().UTC()
	item := &types.SecretIntake{ID: "sci_" + system.GenerateUUID(), ProjectID: projectID, CustomerID: input.CustomerID, ConversationID: input.ConversationID, Title: input.Title, Description: input.Description, BrandName: input.BrandName, AccentColor: input.AccentColor, Fields: input.Fields, ArtifactID: input.ArtifactID, ArtifactBefore: artifactBefore, ArtifactAfter: artifactAfter, Status: "pending", InvitationHash: portalHash(token), InvitationExpiresAt: now.Add(portalInvitationTTL), CreatedAt: now, UpdatedAt: now}
	if err := s.Store.CreateSecretIntake(ctx, item); err != nil {
		return SecretIntakeView{}, "", err
	}
	return secretIntakeResponse(item), strings.TrimRight(s.Cfg.WebServer.URL, "/") + "/connect/intake#" + token, nil
}

func (s *HelixAPIServer) registerSecretIntakeRoutes(router, authRouter *mux.Router) {
	router.HandleFunc("/connect/intake", s.serveSecretIntake).Methods(http.MethodGet)
	router.HandleFunc("/connect/intake/boot.js", s.secretIntakeBoot).Methods(http.MethodGet)
	router.HandleFunc("/connect/intake/redeem", s.redeemSecretIntake).Methods(http.MethodPost)
	router.HandleFunc("/connect/intake/submit", s.submitSecretIntakeForm).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/secret-intakes", s.createSecretIntakeHTTP).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/secret-intakes/{intake_id}", s.getSecretIntake).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/secret-intakes/{intake_id}", s.revokeSecretIntake).Methods(http.MethodDelete)
	authRouter.HandleFunc("/projects/{id}/secret-intakes/{intake_id}/submissions", s.submitSecretIntakeAPI).Methods(http.MethodPost)
}

// @Summary Create a secret intake
// @Description Creates a short-lived, one-time link for collecting requested fields outside chat. The response never contains submitted values.
// @Tags Secret Intakes
// @Accept json
// @Produce json
// @Param id path string true "Project ID"
// @Param request body types.SecretIntakeCreateRequest true "Field schema and branding"
// @Success 201 {object} SecretIntakeCreateResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Router /api/v1/projects/{id}/secret-intakes [post]
// @Security BearerAuth
func (s *HelixAPIServer) createSecretIntakeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.secretIntakeEnabled() {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	project, herr := s.requireProjectAccess(r, types.ActionCreate)
	if herr != nil {
		http.Error(w, herr.Message, herr.StatusCode)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var input secretIntakeInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	view, link, err := s.createSecretIntake(r.Context(), project.ID, input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResponse(w, SecretIntakeCreateResponse{Intake: view, InviteURL: link}, http.StatusCreated)
}

func (s *HelixAPIServer) secretIntakeForProject(w http.ResponseWriter, r *http.Request, action types.Action) (*types.SecretIntake, bool) {
	if !s.secretIntakeEnabled() {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return nil, false
	}
	project, herr := s.requireProjectAccess(r, action)
	if herr != nil {
		http.Error(w, herr.Message, herr.StatusCode)
		return nil, false
	}
	item, err := s.Store.GetSecretIntake(r.Context(), project.ID, mux.Vars(r)["intake_id"])
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			http.Error(w, "unable to load intake", http.StatusInternalServerError)
			return nil, false
		}
		http.Error(w, "intake not found", http.StatusNotFound)
		return nil, false
	}
	return item, true
}

// @Summary Get secret intake status
// @Description Returns metadata and status only; submitted values remain inaccessible through this endpoint.
// @Tags Secret Intakes
// @Produce json
// @Param id path string true "Project ID"
// @Param intake_id path string true "Intake ID"
// @Success 200 {object} SecretIntakeView
// @Failure 404 {object} types.APIError
// @Router /api/v1/projects/{id}/secret-intakes/{intake_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getSecretIntake(w http.ResponseWriter, r *http.Request) {
	item, ok := s.secretIntakeForProject(w, r, types.ActionGet)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResponse(w, secretIntakeResponse(item), http.StatusOK)
}

// @Summary Revoke a secret intake
// @Description Invalidates its link and clears any submitted ciphertext.
// @Tags Secret Intakes
// @Param id path string true "Project ID"
// @Param intake_id path string true "Intake ID"
// @Success 204
// @Failure 404 {object} types.APIError
// @Router /api/v1/projects/{id}/secret-intakes/{intake_id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) revokeSecretIntake(w http.ResponseWriter, r *http.Request) {
	item, ok := s.secretIntakeForProject(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	if err := s.Store.RevokeSecretIntake(r.Context(), item.ProjectID, item.ID); err != nil {
		http.Error(w, "unable to revoke intake", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func secretIntakeValues(fields []types.SecretIntakeField, values map[string]string) (map[string]string, error) {
	if len(values) > len(fields) {
		return nil, errors.New("unexpected field")
	}
	valid := make(map[string]string, len(fields))
	for _, field := range fields {
		value, ok := values[field.Name]
		if field.Required && (!ok || strings.TrimSpace(value) == "") {
			return nil, errors.New("required field missing")
		}
		if len(value) > 4096 {
			return nil, errors.New("field value too long")
		}
		if ok {
			valid[field.Name] = value
		}
	}
	if len(valid) != len(values) {
		return nil, errors.New("unexpected field")
	}
	return valid, nil
}

func (s *HelixAPIServer) saveSecretIntake(ctx context.Context, item *types.SecretIntake, values map[string]string) error {
	valid, err := secretIntakeValues(item.Fields, values)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(valid)
	if err != nil {
		return err
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		return err
	}
	cipher, err := helixcrypto.EncryptAES256GCM(raw, key)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	saved, err := s.Store.SubmitSecretIntake(ctx, item.ProjectID, item.ID, cipher, now, now.Add(secretIntakeValuesTTL))
	if err != nil {
		return err
	}
	if !saved {
		return errors.New("intake already submitted or revoked")
	}
	return nil
}

// @Summary Submit secret intake values
// @Description Write-only project API for trusted integrations. Values are encrypted and cannot be read through the API.
// @Tags Secret Intakes
// @Accept json
// @Param id path string true "Project ID"
// @Param intake_id path string true "Intake ID"
// @Param request body SecretIntakeSubmissionRequest true "Submitted field values"
// @Success 204
// @Failure 400 {object} types.APIError
// @Failure 409 {object} types.APIError
// @Router /api/v1/projects/{id}/secret-intakes/{intake_id}/submissions [post]
// @Security BearerAuth
func (s *HelixAPIServer) submitSecretIntakeAPI(w http.ResponseWriter, r *http.Request) {
	item, ok := s.secretIntakeForProject(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	if secretIntakeStatus(item) != "pending" {
		http.Error(w, "intake unavailable", http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var input SecretIntakeSubmissionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	if err := s.saveSecretIntake(r.Context(), item, input.Values); err != nil {
		http.Error(w, "invalid or unavailable submission", http.StatusBadRequest)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

func (s *HelixAPIServer) secretIntakeFlow(r *http.Request) (*types.SecretIntake, error) {
	cookie, err := r.Cookie(secretIntakeCookie)
	if err != nil || len(cookie.Value) != 43 {
		return nil, store.ErrNotFound
	}
	return s.Store.GetSecretIntakeByFlow(r.Context(), portalHash(cookie.Value), time.Now().UTC())
}

// @Summary Redeem a secret intake invitation
// @Description Exchanges a one-time invitation token for a short-lived browser flow cookie.
// @Tags Secret Intakes
// @Accept json
// @Param request body ConnectRedeemRequest true "Invitation token"
// @Success 204
// @Failure 410 {object} types.APIError
// @Router /connect/intake/redeem [post]
func (s *HelixAPIServer) redeemSecretIntake(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	if !s.secretIntakeEnabled() {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	var input ConnectRedeemRequest
	if json.NewDecoder(r.Body).Decode(&input) != nil || len(input.Token) != 43 {
		http.Error(w, "invalid invitation", http.StatusBadRequest)
		return
	}
	flow, err := portalRandomToken()
	if err != nil {
		http.Error(w, "unable to open intake", http.StatusInternalServerError)
		return
	}
	csrf, err := portalRandomToken()
	if err != nil {
		http.Error(w, "unable to open intake", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	redeemed, err := s.Store.RedeemSecretIntakeInvitation(r.Context(), portalHash(input.Token), portalHash(flow), portalHash(csrf), now, now.Add(portalFlowTTL))
	if err != nil {
		http.Error(w, "unable to open intake", http.StatusInternalServerError)
		return
	}
	if !redeemed {
		http.Error(w, "invitation is expired or already used", http.StatusGone)
		return
	}
	secure := strings.HasPrefix(s.Cfg.WebServer.URL, "https://")
	http.SetCookie(w, &http.Cookie{Name: secretIntakeCookie, Value: flow, Path: "/connect/intake", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(portalFlowTTL.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: secretIntakeCSRFCookie, Value: csrf, Path: "/connect/intake", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(portalFlowTTL.Seconds())})
	w.WriteHeader(http.StatusNoContent)
}

func (s *HelixAPIServer) submitSecretIntakeForm(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	if !s.secretIntakeEnabled() {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	item, err := s.secretIntakeFlow(r)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "unable to load intake", http.StatusInternalServerError)
		return
	}
	if err != nil || secretIntakeStatus(item) != "pending" {
		http.Error(w, "intake unavailable", http.StatusGone)
		return
	}
	csrf, err := r.Cookie(secretIntakeCSRFCookie)
	if err != nil || portalHash(csrf.Value) != item.CSRFHash || r.PostFormValue("csrf") != csrf.Value {
		http.Error(w, "invalid form", http.StatusForbidden)
		return
	}
	values := map[string]string{}
	for name, list := range r.PostForm {
		if name == "csrf" {
			continue
		}
		if len(list) != 1 {
			http.Error(w, "invalid form", http.StatusBadRequest)
			return
		}
		values[name] = list[0]
	}
	if err := s.saveSecretIntake(r.Context(), item, values); err != nil {
		http.Error(w, "invalid or unavailable submission", http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/connect/intake", http.StatusSeeOther)
}

type secretIntakePageData struct {
	Stage, Brand, Accent, Title, Description, CSRF string
	Fields                                         []types.SecretIntakeField
	ArtifactBefore, ArtifactAfter                  template.HTML
	HasArtifact                                    bool
}

//go:embed templates/secret_intake.html
var secretIntakeTemplateText string

var secretIntakePage = template.Must(template.New("secret-intake").Parse(secretIntakeTemplateText))

func (s *HelixAPIServer) serveSecretIntake(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	if !s.secretIntakeEnabled() {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	data := secretIntakePageData{Stage: "landing", Brand: "Helix Connect", Accent: "#00b8d4", Title: "Secure connection"}
	item, err := s.secretIntakeFlow(r)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		http.Error(w, "unable to load intake", http.StatusInternalServerError)
		return
	}
	if err == nil {
		data.Brand, data.Accent, data.Title, data.Description, data.Fields = item.BrandName, item.AccentColor, item.Title, item.Description, item.Fields
		data.ArtifactBefore, data.ArtifactAfter = template.HTML(item.ArtifactBefore), template.HTML(item.ArtifactAfter)
		data.HasArtifact = item.ArtifactID != ""
		data.Stage = secretIntakeStatus(item)
		if data.Stage == "pending" {
			csrf, err := r.Cookie(secretIntakeCSRFCookie)
			if err == nil && portalHash(csrf.Value) == item.CSRFHash {
				data.CSRF = csrf.Value
			}
			if data.CSRF == "" {
				data.Stage = "expired"
			}
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = secretIntakePage.Execute(w, data)
}

func (s *HelixAPIServer) secretIntakeBoot(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(`(()=>{const token=location.hash.slice(1);if(!token)return;history.replaceState(null,"",location.pathname);const button=document.getElementById("redeem");if(!button)return;button.hidden=false;button.addEventListener("click",async()=>{button.disabled=true;try{const res=await fetch("/connect/intake/redeem",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token}),credentials:"same-origin"});if(!res.ok)throw new Error();location.reload()}catch{document.getElementById("state").textContent="This link is expired or already used.";button.hidden=true}})})();`))
}

func (s *HelixAPIServer) runSecretIntakeReaper(ctx context.Context) {
	if !s.secretIntakeEnabled() {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.Store.ReapExpiredSecretIntakes(ctx, time.Now().UTC()); err != nil {
			log.Error().Err(err).Msg("reap expired secret intakes")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ConsumeSecretIntake clears the ciphertext atomically, then passes the values to trusted backend code once.
// Callbacks must not serialize values into tool results, chat messages, or logs.
func (s *HelixAPIServer) ConsumeSecretIntake(ctx context.Context, projectID, intakeID string, consume func(map[string]string) error) error {
	if !s.secretIntakeEnabled() {
		return errors.New("secret intake is disabled")
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		return err
	}
	cipher, err := s.Store.TakeSecretIntake(ctx, projectID, intakeID, time.Now().UTC())
	if err != nil {
		return err
	}
	raw, err := helixcrypto.DecryptAES256GCM(cipher, key)
	if err != nil {
		return err
	}
	var values map[string]string
	if err := json.Unmarshal(raw, &values); err != nil {
		return err
	}
	if err := consume(values); err != nil {
		return fmt.Errorf("consume intake: %w", err)
	}
	return nil
}
