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
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const secretIntakeCookie = "helix_secret_intake_flow"
const secretIntakeCSRFCookie = "helix_secret_intake_csrf"
const secretIntakeValuesTTL = time.Hour

var secretIntakeFieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
var secretIntakeAutocomplete = map[string]bool{"off": true, "username": true, "current-password": true, "new-password": true, "one-time-code": true}

type secretIntakeInput = types.SecretIntakeCreateRequest

type secretIntakeView struct {
	ID              string                    `json:"id"`
	ProjectID       string                    `json:"project_id"`
	CustomerID      string                    `json:"customer_id"`
	ConversationID  string                    `json:"conversation_id"`
	Status          string                    `json:"status"`
	Fields          []types.SecretIntakeField `json:"fields"`
	ExpiresAt       time.Time                 `json:"expires_at"`
	ValuesExpiresAt *time.Time                `json:"values_expires_at,omitempty"`
}

func (s *HelixAPIServer) secretIntakeDB() (*gorm.DB, bool) {
	accessor, ok := s.Store.(interface{ GormDB() *gorm.DB })
	if !ok || !s.Cfg.SecretIntakeEnabled || os.Getenv("HELIX_ENCRYPTION_KEY") == "" || strings.TrimSpace(s.Cfg.WebServer.URL) == "" {
		return nil, false
	}
	if _, err := s.getEncryptionKey(); err != nil {
		return nil, false
	}
	return accessor.GormDB(), true
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

func secretIntakeResponse(item *types.SecretIntake) secretIntakeView {
	view := secretIntakeView{ID: item.ID, ProjectID: item.ProjectID, CustomerID: item.CustomerID, ConversationID: item.ConversationID, Status: secretIntakeStatus(item), Fields: item.Fields, ExpiresAt: item.InvitationExpiresAt}
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

func (s *HelixAPIServer) createSecretIntake(ctx context.Context, projectID string, input secretIntakeInput) (secretIntakeView, string, error) {
	db, ok := s.secretIntakeDB()
	if !ok {
		return secretIntakeView{}, "", errors.New("secret intake is disabled")
	}
	if err := validateSecretIntakeInput(&input); err != nil {
		return secretIntakeView{}, "", err
	}
	baseURL, err := url.Parse(s.Cfg.WebServer.URL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "https" && !(baseURL.Scheme == "http" && (baseURL.Hostname() == "localhost" || baseURL.Hostname() == "127.0.0.1" || baseURL.Hostname() == "::1"))) {
		return secretIntakeView{}, "", errors.New("server URL must use HTTPS")
	}
	var artifactBefore, artifactAfter string
	if input.ArtifactID != "" {
		artifactBefore, artifactAfter, err = s.loadSecretIntakeArtifact(ctx, projectID, input.ArtifactID)
		if err != nil {
			return secretIntakeView{}, "", err
		}
	}
	token, err := portalRandomToken()
	if err != nil {
		return secretIntakeView{}, "", err
	}
	now := time.Now().UTC()
	item := &types.SecretIntake{ID: "sci_" + system.GenerateUUID(), ProjectID: projectID, CustomerID: input.CustomerID, ConversationID: input.ConversationID, Title: input.Title, Description: input.Description, BrandName: input.BrandName, AccentColor: input.AccentColor, Fields: input.Fields, ArtifactID: input.ArtifactID, ArtifactBefore: artifactBefore, ArtifactAfter: artifactAfter, Status: "pending", InvitationHash: portalHash(token), InvitationExpiresAt: now.Add(portalInvitationTTL), CreatedAt: now, UpdatedAt: now}
	if err := db.WithContext(ctx).Create(item).Error; err != nil {
		return secretIntakeView{}, "", err
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

func (s *HelixAPIServer) createSecretIntakeHTTP(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.secretIntakeDB(); !ok {
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
	writeResponse(w, map[string]any{"intake": view, "invite_url": link}, http.StatusCreated)
}

func (s *HelixAPIServer) secretIntakeForProject(w http.ResponseWriter, r *http.Request, action types.Action) (*gorm.DB, *types.SecretIntake, bool) {
	db, ok := s.secretIntakeDB()
	if !ok {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return nil, nil, false
	}
	project, herr := s.requireProjectAccess(r, action)
	if herr != nil {
		http.Error(w, herr.Message, herr.StatusCode)
		return nil, nil, false
	}
	var item types.SecretIntake
	if err := db.WithContext(r.Context()).Where("id = ? AND project_id = ?", mux.Vars(r)["intake_id"], project.ID).First(&item).Error; err != nil {
		http.Error(w, "intake not found", http.StatusNotFound)
		return nil, nil, false
	}
	return db, &item, true
}

func (s *HelixAPIServer) getSecretIntake(w http.ResponseWriter, r *http.Request) {
	_, item, ok := s.secretIntakeForProject(w, r, types.ActionGet)
	if !ok {
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResponse(w, secretIntakeResponse(item), http.StatusOK)
}

func (s *HelixAPIServer) revokeSecretIntake(w http.ResponseWriter, r *http.Request) {
	db, item, ok := s.secretIntakeForProject(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	if err := db.WithContext(r.Context()).Model(&types.SecretIntake{}).Where("id = ? AND project_id = ?", item.ID, item.ProjectID).Updates(map[string]any{"status": "revoked", "values_encrypted": "", "invitation_hash": "", "flow_hash": "", "csrf_hash": ""}).Error; err != nil {
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
	result := s.Store.(interface{ GormDB() *gorm.DB }).GormDB().WithContext(ctx).Model(&types.SecretIntake{}).Where("id = ? AND project_id = ? AND status = ?", item.ID, item.ProjectID, "pending").Updates(map[string]any{"status": "submitted", "values_encrypted": cipher, "values_expires_at": now.Add(secretIntakeValuesTTL), "invitation_hash": ""})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("intake already submitted or revoked")
	}
	return nil
}

func (s *HelixAPIServer) submitSecretIntakeAPI(w http.ResponseWriter, r *http.Request) {
	_, item, ok := s.secretIntakeForProject(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	if secretIntakeStatus(item) != "pending" {
		http.Error(w, "intake unavailable", http.StatusConflict)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	var input struct {
		Values map[string]string `json:"values"`
	}
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

func secretIntakeFlow(db *gorm.DB, r *http.Request) (*types.SecretIntake, error) {
	cookie, err := r.Cookie(secretIntakeCookie)
	if err != nil || len(cookie.Value) != 43 {
		return nil, gorm.ErrRecordNotFound
	}
	var item types.SecretIntake
	err = db.WithContext(r.Context()).Where("flow_hash = ? AND flow_expires_at > ?", portalHash(cookie.Value), time.Now().UTC()).First(&item).Error
	return &item, err
}

func (s *HelixAPIServer) redeemSecretIntake(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	db, ok := s.secretIntakeDB()
	if !ok {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 256)
	var input struct {
		Token string `json:"token"`
	}
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
	result := db.WithContext(r.Context()).Model(&types.SecretIntake{}).Where("invitation_hash = ? AND status = ? AND invitation_expires_at > ?", portalHash(input.Token), "pending", now).Updates(map[string]any{"invitation_hash": "", "flow_hash": portalHash(flow), "csrf_hash": portalHash(csrf), "flow_expires_at": now.Add(portalFlowTTL)})
	if result.Error != nil || result.RowsAffected != 1 {
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
	db, ok := s.secretIntakeDB()
	if !ok {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16384)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	item, err := secretIntakeFlow(db, r)
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
	db, ok := s.secretIntakeDB()
	if !ok {
		http.Error(w, "secret intake is disabled", http.StatusNotImplemented)
		return
	}
	data := secretIntakePageData{Stage: "landing", Brand: "Helix Connect", Accent: "#00b8d4", Title: "Secure connection"}
	item, err := secretIntakeFlow(db, r)
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
	db, ok := s.secretIntakeDB()
	if !ok {
		return
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		reapExpiredSecretIntakes(ctx, db, time.Now().UTC())
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func reapExpiredSecretIntakes(ctx context.Context, db *gorm.DB, now time.Time) {
	if err := db.WithContext(ctx).Model(&types.SecretIntake{}).Where("status = ? AND values_expires_at <= ?", "submitted", now).Updates(map[string]any{"status": "expired", "values_encrypted": "", "flow_hash": "", "csrf_hash": ""}).Error; err != nil {
		log.Error().Err(err).Msg("reap expired secret intake values")
	}
	if err := db.WithContext(ctx).Model(&types.SecretIntake{}).Where("status = ? AND flow_expires_at <= ? AND flow_hash <> ?", "pending", now, "").Updates(map[string]any{"status": "expired", "invitation_hash": "", "flow_hash": "", "csrf_hash": ""}).Error; err != nil {
		log.Error().Err(err).Msg("reap expired secret intake flows")
	}
	if err := db.WithContext(ctx).Model(&types.SecretIntake{}).Where("status = ? AND invitation_expires_at <= ? AND flow_hash = ?", "pending", now, "").Updates(map[string]any{"status": "expired", "invitation_hash": ""}).Error; err != nil {
		log.Error().Err(err).Msg("reap expired secret intake invitations")
	}
}

// ConsumeSecretIntake passes values to trusted backend code once, then clears the ciphertext.
// Callbacks must not serialize values into tool results, chat messages, or logs.
func (s *HelixAPIServer) ConsumeSecretIntake(ctx context.Context, projectID, intakeID string, consume func(map[string]string) error) error {
	db, ok := s.secretIntakeDB()
	if !ok {
		return errors.New("secret intake is disabled")
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var item types.SecretIntake
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND project_id = ?", intakeID, projectID).First(&item).Error; err != nil {
			return err
		}
		if secretIntakeStatus(&item) != "submitted" {
			return errors.New("intake unavailable")
		}
		key, err := s.getEncryptionKey()
		if err != nil {
			return err
		}
		raw, err := helixcrypto.DecryptAES256GCM(item.ValuesEncrypted, key)
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
		result := tx.Model(&types.SecretIntake{}).Where("id = ? AND status = ?", item.ID, "submitted").Updates(map[string]any{"status": "consumed", "values_encrypted": ""})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("intake already consumed")
		}
		return nil
	})
}
