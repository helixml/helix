package server

import (
	"crypto/rand"
	"crypto/sha256"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
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
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	portalInvitationTTL = 10 * time.Minute
	portalFlowTTL       = 20 * time.Minute
	portalSessionTTL    = time.Hour
	portalFlowCookie    = "helix_portal_flow"
)

var portalAccentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *HelixAPIServer) registerPortalConnectionRoutes(router, authRouter *mux.Router) {
	router.HandleFunc("/connect", s.servePortalConnection).Methods(http.MethodGet)
	router.HandleFunc("/connect/boot.js", s.portalBootScript).Methods(http.MethodGet)
	router.HandleFunc("/connect/redeem", s.redeemPortalConnection).Methods(http.MethodPost)
	router.HandleFunc("/connect/password", s.submitPortalPassword).Methods(http.MethodPost)
	router.HandleFunc("/connect/otp", s.submitPortalOTP).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/portal-connections", s.createPortalConnection).Methods(http.MethodPost)
	authRouter.HandleFunc("/projects/{id}/portal-connections/{connection_id}", s.getPortalConnection).Methods(http.MethodGet)
	authRouter.HandleFunc("/projects/{id}/portal-connections/{connection_id}", s.revokePortalConnection).Methods(http.MethodDelete)
	authRouter.HandleFunc("/projects/{id}/portal-connections/{connection_id}/account-status", s.mockPortalAccountStatus).Methods(http.MethodGet)
}

type createPortalConnectionRequest struct {
	CustomerID     string `json:"customer_id"`
	ConversationID string `json:"conversation_id"`
	Portal         string `json:"portal"`
	BrandName      string `json:"brand_name"`
	AccentColor    string `json:"accent_color"`
}

type portalConnectionView struct {
	ID               string                       `json:"id"`
	ProjectID        string                       `json:"project_id"`
	CustomerID       string                       `json:"customer_id"`
	ConversationID   string                       `json:"conversation_id"`
	Portal           string                       `json:"portal"`
	Status           types.PortalConnectionStatus `json:"status"`
	ExpiresAt        time.Time                    `json:"expires_at"`
	SessionExpiresAt *time.Time                   `json:"session_expires_at,omitempty"`
}

func (s *HelixAPIServer) portalConnectionEnabled() bool {
	return s.Cfg.PortalMockEnabled && os.Getenv("HELIX_ENCRYPTION_KEY") != ""
}

func (s *HelixAPIServer) portalConnectionDB() (*gorm.DB, bool) {
	accessor, ok := s.Store.(interface{ GormDB() *gorm.DB })
	if !ok || !s.portalConnectionEnabled() {
		return nil, false
	}
	return accessor.GormDB(), true
}

func portalRandomToken() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

func portalHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func portalView(attempt *types.PortalConnectionAttempt) portalConnectionView {
	status := attempt.Status
	now := time.Now().UTC()
	if status == types.PortalConnectionConnected && now.After(attempt.SessionExpiresAt) {
		status = types.PortalConnectionExpired
	} else if status != types.PortalConnectionConnected && status != types.PortalConnectionRevoked && status != types.PortalConnectionFailed && !attempt.FlowExpiresAt.IsZero() && now.After(attempt.FlowExpiresAt) {
		status = types.PortalConnectionExpired
	} else if status != types.PortalConnectionConnected && status != types.PortalConnectionRevoked && status != types.PortalConnectionFailed && attempt.FlowExpiresAt.IsZero() && now.After(attempt.InvitationExpiresAt) {
		status = types.PortalConnectionExpired
	}
	view := portalConnectionView{
		ID: attempt.ID, ProjectID: attempt.ProjectID, CustomerID: attempt.CustomerID,
		ConversationID: attempt.ConversationID, Portal: attempt.Portal, Status: status,
		ExpiresAt: attempt.InvitationExpiresAt,
	}
	if !attempt.SessionExpiresAt.IsZero() {
		view.SessionExpiresAt = &attempt.SessionExpiresAt
	}
	return view
}

// createPortalConnection creates a mock-only credential handoff invitation.
// The bearer token is returned once, in the URL fragment so HTTP logs never see it.
func (s *HelixAPIServer) createPortalConnection(w http.ResponseWriter, r *http.Request) {
	db, ok := s.portalConnectionDB()
	if !ok {
		http.Error(w, "portal connection demo is disabled", http.StatusNotImplemented)
		return
	}
	project, herr := s.requireProjectAccess(r, types.ActionCreate)
	if herr != nil {
		http.Error(w, herr.Message, herr.StatusCode)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input createPortalConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	input.CustomerID = strings.TrimSpace(input.CustomerID)
	input.ConversationID = strings.TrimSpace(input.ConversationID)
	input.BrandName = strings.TrimSpace(input.BrandName)
	if input.CustomerID == "" || len(input.CustomerID) > 128 || input.ConversationID == "" || len(input.ConversationID) > 128 {
		http.Error(w, "customer_id and conversation_id are required (max 128 characters)", http.StatusBadRequest)
		return
	}
	if input.Portal != "mock" {
		http.Error(w, "only the mock portal is available", http.StatusBadRequest)
		return
	}
	if input.BrandName == "" {
		input.BrandName = "Helix Connect"
	}
	if len(input.BrandName) > 80 {
		http.Error(w, "brand_name is too long", http.StatusBadRequest)
		return
	}
	if input.AccentColor == "" {
		input.AccentColor = "#00b8d4"
	}
	if !portalAccentPattern.MatchString(input.AccentColor) {
		http.Error(w, "accent_color must be a six-digit hex color", http.StatusBadRequest)
		return
	}
	baseURL, err := url.Parse(s.Cfg.WebServer.URL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "https" && !(baseURL.Scheme == "http" && (baseURL.Hostname() == "localhost" || baseURL.Hostname() == "127.0.0.1" || baseURL.Hostname() == "::1"))) {
		http.Error(w, "server URL is invalid", http.StatusInternalServerError)
		return
	}
	token, err := portalRandomToken()
	if err != nil {
		http.Error(w, "unable to create invitation", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	attempt := &types.PortalConnectionAttempt{
		ID: "pca_" + system.GenerateUUID(), ProjectID: project.ID,
		CustomerID: input.CustomerID, ConversationID: input.ConversationID,
		Portal: input.Portal, BrandName: input.BrandName, AccentColor: input.AccentColor,
		Status: types.PortalConnectionPasswordPending, InvitationHash: portalHash(token),
		InvitationExpiresAt: now.Add(portalInvitationTTL), CreatedAt: now, UpdatedAt: now,
	}
	if err := db.WithContext(r.Context()).Create(attempt).Error; err != nil {
		http.Error(w, "unable to create invitation", http.StatusInternalServerError)
		return
	}
	link := strings.TrimRight(s.Cfg.WebServer.URL, "/") + "/connect#" + token
	w.Header().Set("Cache-Control", "no-store")
	writeResponse(w, map[string]any{"connection": portalView(attempt), "invite_url": link}, http.StatusCreated)
}

func (s *HelixAPIServer) portalAttemptForProject(w http.ResponseWriter, r *http.Request, action types.Action) (*gorm.DB, *types.PortalConnectionAttempt, bool) {
	db, ok := s.portalConnectionDB()
	if !ok {
		http.Error(w, "portal connection demo is disabled", http.StatusNotImplemented)
		return nil, nil, false
	}
	project, herr := s.requireProjectAccess(r, action)
	if herr != nil {
		http.Error(w, herr.Message, herr.StatusCode)
		return nil, nil, false
	}
	var attempt types.PortalConnectionAttempt
	if err := db.WithContext(r.Context()).Where("id = ? AND project_id = ?", mux.Vars(r)["connection_id"], project.ID).First(&attempt).Error; err != nil {
		http.Error(w, "connection not found", http.StatusNotFound)
		return nil, nil, false
	}
	return db, &attempt, true
}

func (s *HelixAPIServer) getPortalConnection(w http.ResponseWriter, r *http.Request) {
	_, attempt, ok := s.portalAttemptForProject(w, r, types.ActionGet)
	if ok {
		w.Header().Set("Cache-Control", "no-store")
		writeResponse(w, portalView(attempt), http.StatusOK)
	}
}

func (s *HelixAPIServer) revokePortalConnection(w http.ResponseWriter, r *http.Request) {
	db, attempt, ok := s.portalAttemptForProject(w, r, types.ActionUpdate)
	if !ok {
		return
	}
	if err := db.WithContext(r.Context()).Model(&types.PortalConnectionAttempt{}).Where("id = ? AND project_id = ?", attempt.ID, attempt.ProjectID).Updates(map[string]any{
		"status": types.PortalConnectionRevoked, "session_encrypted": "", "flow_hash": "", "csrf_hash": "", "invitation_hash": "",
	}).Error; err != nil {
		http.Error(w, "unable to revoke connection", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mockPortalAccountStatus is the bounded operation used to verify that the
// backend, rather than the model, owns the authenticated portal session.
func (s *HelixAPIServer) mockPortalAccountStatus(w http.ResponseWriter, r *http.Request) {
	_, attempt, ok := s.portalAttemptForProject(w, r, types.ActionGet)
	if !ok {
		return
	}
	if portalView(attempt).Status != types.PortalConnectionConnected {
		http.Error(w, "connection is not active", http.StatusConflict)
		return
	}
	key, err := s.getEncryptionKey()
	if err != nil {
		http.Error(w, "connection unavailable", http.StatusInternalServerError)
		return
	}
	if _, err := helixcrypto.DecryptAES256GCM(attempt.SessionEncrypted, key); err != nil {
		http.Error(w, "connection unavailable", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeResponse(w, map[string]any{"customer_id": attempt.CustomerID, "portal": "mock", "account_status": "active"}, http.StatusOK)
}

func portalPageHeaders(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'unsafe-inline'; connect-src 'self'; form-action 'self'; base-uri 'none'; frame-ancestors 'none'")
}

func portalFlowAttempt(db *gorm.DB, r *http.Request) (*types.PortalConnectionAttempt, error) {
	cookie, err := r.Cookie(portalFlowCookie)
	if err != nil || len(cookie.Value) < 32 {
		return nil, gorm.ErrRecordNotFound
	}
	var attempt types.PortalConnectionAttempt
	err = db.WithContext(r.Context()).Where("flow_hash = ? AND flow_expires_at > ?", portalHash(cookie.Value), time.Now().UTC()).First(&attempt).Error
	return &attempt, err
}

func (s *HelixAPIServer) servePortalConnection(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	db, ok := s.portalConnectionDB()
	if !ok {
		http.Error(w, "portal connection demo is disabled", http.StatusNotImplemented)
		return
	}
	attempt, err := portalFlowAttempt(db, r)
	if err != nil {
		portalRender(w, portalPageData{Title: "Connect a demo portal", Stage: "landing", Brand: "Helix Connect", Accent: "#00b8d4"})
		return
	}
	view := portalView(attempt)
	stage := string(view.Status)
	if stage == string(types.PortalConnectionExpired) || stage == string(types.PortalConnectionRevoked) {
		stage = "unavailable"
	}
	csrf, _ := r.Cookie("helix_portal_csrf")
	csrfValue := ""
	if csrf != nil && portalHash(csrf.Value) == attempt.CSRFHash {
		csrfValue = csrf.Value
	}
	message := ""
	if r.URL.Query().Get("error") == "invalid" && (stage == "password_pending" || stage == "otp_pending") {
		message = "Those demo details were not accepted. Please try again."
	}
	portalRender(w, portalPageData{Title: "Connect a demo portal", Stage: stage, Brand: attempt.BrandName, Accent: attempt.AccentColor, CSRF: csrfValue, Message: message})
}

func (s *HelixAPIServer) redeemPortalConnection(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	db, ok := s.portalConnectionDB()
	if !ok {
		http.Error(w, "portal connection demo is disabled", http.StatusNotImplemented)
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
		http.Error(w, "unable to open connection", http.StatusInternalServerError)
		return
	}
	csrf, err := portalRandomToken()
	if err != nil {
		http.Error(w, "unable to open connection", http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	result := db.WithContext(r.Context()).Model(&types.PortalConnectionAttempt{}).
		Where("invitation_hash = ? AND status = ? AND invitation_expires_at > ?", portalHash(input.Token), types.PortalConnectionPasswordPending, now).
		Updates(map[string]any{"invitation_hash": "", "flow_hash": portalHash(flow), "csrf_hash": portalHash(csrf), "flow_expires_at": now.Add(portalFlowTTL)})
	if result.Error != nil || result.RowsAffected != 1 {
		http.Error(w, "invitation is expired or already used", http.StatusGone)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: portalFlowCookie, Value: flow, Path: "/connect", HttpOnly: true, Secure: strings.HasPrefix(s.Cfg.WebServer.URL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: int(portalFlowTTL.Seconds())})
	// A second cookie carries the CSRF token for the server-rendered form. It
	// is not a credential and cannot authenticate without the HttpOnly flow.
	http.SetCookie(w, &http.Cookie{Name: "helix_portal_csrf", Value: csrf, Path: "/connect", HttpOnly: true, Secure: strings.HasPrefix(s.Cfg.WebServer.URL, "https://"), SameSite: http.SameSiteStrictMode, MaxAge: int(portalFlowTTL.Seconds())})
	w.WriteHeader(http.StatusNoContent)
}

func portalCheckCSRF(r *http.Request, attempt *types.PortalConnectionAttempt) bool {
	cookie, err := r.Cookie("helix_portal_csrf")
	if err != nil || cookie.Value == "" {
		return false
	}
	return portalHash(cookie.Value) == attempt.CSRFHash && r.PostFormValue("csrf") == cookie.Value
}

func (s *HelixAPIServer) submitPortalPassword(w http.ResponseWriter, r *http.Request) {
	s.portalSubmit(w, r, false)
}

func (s *HelixAPIServer) submitPortalOTP(w http.ResponseWriter, r *http.Request) {
	s.portalSubmit(w, r, true)
}

func (s *HelixAPIServer) portalSubmit(w http.ResponseWriter, r *http.Request, otpStage bool) {
	portalPageHeaders(w)
	db, ok := s.portalConnectionDB()
	if !ok {
		http.Error(w, "portal connection demo is disabled", http.StatusNotImplemented)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	attempt, err := portalFlowAttempt(db, r)
	if err != nil || !portalCheckCSRF(r, attempt) {
		http.Error(w, "connection form expired", http.StatusForbidden)
		return
	}
	expected := types.PortalConnectionPasswordPending
	if otpStage {
		expected = types.PortalConnectionOTPPending
	}
	if attempt.Status != expected || portalView(attempt).Status == types.PortalConnectionExpired {
		http.Error(w, "connection step unavailable", http.StatusConflict)
		return
	}
	username, password, otp := r.PostFormValue("username"), r.PostFormValue("password"), r.PostFormValue("otp")
	if (!otpStage && (len(username) > 128 || len(password) > 256)) || (otpStage && len(otp) > 32) {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	var responseStatus int
	err = db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		var current types.PortalConnectionAttempt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", attempt.ID).First(&current).Error; err != nil {
			return err
		}
		if current.Status != expected || time.Now().UTC().After(current.FlowExpiresAt) {
			responseStatus = http.StatusConflict
			return nil
		}
		if otpStage {
			current.OTPAttempts++
			if otp == "123456" {
				session, err := portalRandomToken()
				if err != nil {
					return err
				}
				key, err := s.getEncryptionKey()
				if err != nil {
					return err
				}
				current.SessionEncrypted, err = helixcrypto.EncryptAES256GCM([]byte(session), key)
				if err != nil {
					return err
				}
				current.SessionExpiresAt = time.Now().UTC().Add(portalSessionTTL)
				current.Status = types.PortalConnectionConnected
			} else if current.OTPAttempts >= 5 {
				current.Status = types.PortalConnectionFailed
			}
		} else {
			current.PasswordAttempts++
			if username == "demo" && password == "demo-password" {
				current.Status = types.PortalConnectionOTPPending
			} else if current.PasswordAttempts >= 3 {
				current.Status = types.PortalConnectionFailed
			}
		}
		if err := tx.Save(&current).Error; err != nil {
			return err
		}
		if current.Status == expected || current.Status == types.PortalConnectionFailed {
			responseStatus = http.StatusUnauthorized
		}
		return nil
	})
	if err != nil {
		http.Error(w, "unable to process connection", http.StatusInternalServerError)
		return
	}
	if responseStatus != 0 {
		if responseStatus == http.StatusUnauthorized {
			http.Redirect(w, r, "/connect?error=invalid", http.StatusSeeOther)
			return
		}
		http.Error(w, "connection step unavailable", responseStatus)
		return
	}
	http.Redirect(w, r, "/connect", http.StatusSeeOther)
}

type portalPageData struct {
	Title, Stage, Brand, Accent, CSRF, Message string
}

//go:embed templates/portal_connection.html
var portalTemplateText string

var portalPage = template.Must(template.New("portal").Parse(portalTemplateText))

func portalRender(w http.ResponseWriter, data portalPageData) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_ = portalPage.Execute(w, data)
}

func (s *HelixAPIServer) portalBootScript(w http.ResponseWriter, r *http.Request) {
	portalPageHeaders(w)
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(`(()=>{const token=location.hash.slice(1);if(!token)return;history.replaceState(null,"",location.pathname);const button=document.getElementById("redeem");if(!button)return;button.hidden=false;button.addEventListener("click",async()=>{button.disabled=true;try{const res=await fetch("/connect/redeem",{method:"POST",headers:{"Content-Type":"application/json"},body:JSON.stringify({token}),credentials:"same-origin"});if(!res.ok)throw new Error();location.reload()}catch{const el=document.getElementById("state");if(el)el.textContent="This connection link is expired or already used.";button.hidden=true}})})();`))
}
