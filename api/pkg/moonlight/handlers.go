package moonlight

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// MoonlightServer provides HTTP/RTSP endpoints for Moonlight protocol
type MoonlightServer struct {
	proxy         *MoonlightProxy
	store         store.Store
	apps          map[uint64]*MoonlightApp // app_id -> app config
	publicURL     string                   // Public URL for this server
	pairedClients map[string]*PairedClient // certificate -> client mapping
	authenticator auth.Authenticator       // Helix auth system
	pairingPINs   map[string]*PairingPIN   // PIN -> pairing info (single-use)
}

// PairingPIN represents a single-use pairing PIN
type PairingPIN struct {
	PIN       string    `json:"pin"`
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Used      bool      `json:"used"`
}

// PairedClient represents a paired Moonlight client
type PairedClient struct {
	UniqueID     string    `json:"unique_id"`
	Certificate  string    `json:"certificate"`
	UserID       string    `json:"user_id"`       // Helix user ID
	PairedAt     time.Time `json:"paired_at"`
	LastActivity time.Time `json:"last_activity"`
}

// MoonlightApp represents a configured app that can be launched
type MoonlightApp struct {
	ID              uint64 `json:"id"`
	Title           string `json:"title"`
	HelixSessionID  string `json:"helix_session_id,omitempty"`
	RunnerID        string `json:"runner_id"`
	IconPNGPath     string `json:"icon_png_path,omitempty"`
	SupportHDR      bool   `json:"support_hdr"`
}

// MoonlightAppListResponse represents the response to /applist
type MoonlightAppListResponse struct {
	StatusCode int                     `xml:"status_code"`
	StatusMessage string               `xml:"status_message"`
	Apps       []*MoonlightAppResponse `xml:"App"`
}

// MoonlightAppResponse represents a single app in the app list
type MoonlightAppResponse struct {
	AppTitle   string `xml:"AppTitle"`
	ID         string `xml:"ID"`
	IsRunning  string `xml:"IsRunning"`
	IconPNG    string `xml:"IconPNG,omitempty"`
	SupportHDR string `xml:"SupportHDR"`
}

// LaunchRequest represents a Moonlight launch request
type LaunchRequest struct {
	AppID     uint64 `json:"appid"`
	Mode      string `json:"mode"`
	AdditionalStderr int `json:"additionalStderr"`
	SurroundAudioInfo int `json:"surroundAudioInfo"`
	RemoteControllersBitmap int `json:"remoteControllersBitmap"`
	GCMEncryption bool `json:"gcmEncryption"`
	ClientRefreshRateX100 int `json:"clientRefreshRateX100"`
	SurroundChannelCount int `json:"surroundChannelCount"`
	ClientColorSpace int `json:"clientColorSpace"`
	ClientColorRange int `json:"clientColorRange"`
	ClientPlaybackHDR int `json:"clientPlaybackHDR"`
	OptimizeGameSettings int `json:"optimizeGameSettings"`
	AudioConfiguration string `json:"audioConfiguration"`
	VideoConfiguration string `json:"videoConfiguration"`
	FrameRate int `json:"frameRate"`
	Width int `json:"width"`
	Height int `json:"height"`
	LocalAudioPlayback int `json:"localAudioPlayback"`
	HostAudio int `json:"hostAudio"`
}

// LaunchResponse represents a Moonlight launch response  
type LaunchResponse struct {
	StatusCode    int    `xml:"status_code"`
	StatusMessage string `xml:"status_message"`
	GfeVersion    string `xml:"gfeversion"`
	GameSession   string `xml:"gamesession"`
	SessionURL0   string `xml:"sessionUrl0"`
}

// NewMoonlightServer creates a new Moonlight HTTP/RTSP server
func NewMoonlightServer(proxy *MoonlightProxy, store store.Store, publicURL string, authenticator auth.Authenticator) *MoonlightServer {
	return &MoonlightServer{
		proxy:         proxy,
		store:         store,
		apps:          make(map[uint64]*MoonlightApp),
		publicURL:     publicURL,
		pairedClients: make(map[string]*PairedClient),
		authenticator: authenticator,
		pairingPINs:   make(map[string]*PairingPIN),
	}
}

// RegisterRoutes adds Moonlight endpoints to the router
func (ms *MoonlightServer) RegisterRoutes(router *mux.Router) {
	// Moonlight API endpoints
	moonlightRouter := router.PathPrefix("/moonlight").Subrouter()
	
	// Standard Moonlight endpoints
	moonlightRouter.HandleFunc("/serverinfo", ms.handleServerInfo).Methods("GET")
	moonlightRouter.HandleFunc("/pair", ms.handlePair).Methods("GET") 
	moonlightRouter.HandleFunc("/applist", ms.authMiddleware(ms.handleAppList)).Methods("GET") 
	moonlightRouter.HandleFunc("/launch", ms.authMiddleware(ms.handleLaunch)).Methods("GET")
	moonlightRouter.HandleFunc("/resume", ms.authMiddleware(ms.handleResume)).Methods("GET")
	moonlightRouter.HandleFunc("/cancel", ms.authMiddleware(ms.handleCancel)).Methods("GET")
	moonlightRouter.HandleFunc("/quit", ms.authMiddleware(ms.handleQuit)).Methods("GET")
	
	// RTSP proxy for session negotiation  
	moonlightRouter.HandleFunc("/rtsp", ms.handleRTSP).Methods("GET", "POST", "SETUP", "PLAY", "TEARDOWN")
	
	// Admin endpoints for managing apps
	adminRouter := moonlightRouter.PathPrefix("/admin").Subrouter()
	adminRouter.HandleFunc("/apps", ms.handleListApps).Methods("GET")
	adminRouter.HandleFunc("/apps", ms.handleCreateApp).Methods("POST")
	adminRouter.HandleFunc("/apps/{app_id}", ms.handleUpdateApp).Methods("PUT")
	adminRouter.HandleFunc("/apps/{app_id}", ms.handleDeleteApp).Methods("DELETE")
	adminRouter.HandleFunc("/refresh", ms.handleRefreshApps).Methods("POST")
	
	log.Info().Msg("Registered Moonlight server routes")
}

// authMiddleware validates Moonlight client certificates against Helix users
func (ms *MoonlightServer) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract certificate from request (Moonlight sends this in various ways)
		cert := r.Header.Get("X-Nvidia-ClientCert")
		if cert == "" {
			cert = r.URL.Query().Get("uniqueid")
		}
		if cert == "" {
			// Try to extract from client certificate or other auth headers
			cert = r.Header.Get("Authorization")
		}
		
		if cert == "" {
			ms.writeErrorResponse(w, 401, "No certificate provided")
			return
		}
		
		// Validate certificate against paired clients
		client, exists := ms.pairedClients[cert]
		if !exists {
			ms.writeErrorResponse(w, 401, "Invalid certificate - device not paired")
			return
		}
		
		// Update last activity
		client.LastActivity = time.Now()
		
		// Add user context to request
		ctx := context.WithValue(r.Context(), "moonlight_user_id", client.UserID)
		ctx = context.WithValue(ctx, "moonlight_client", client)
		
		next(w, r.WithContext(ctx))
	}
}

// handlePair handles Moonlight device pairing with Helix authentication
func (ms *MoonlightServer) handlePair(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	
	// Get PIN from request
	pin := r.URL.Query().Get("pin")
	uniqueID := r.URL.Query().Get("uniqueid")
	
	if pin == "" || uniqueID == "" {
		ms.writeErrorResponse(w, 400, "Missing pin or uniqueid parameter")
		return
	}
	
	// Validate PIN against Helix user session
	// For simplicity, we'll use PIN as a temporary auth token from Helix frontend
	// In production, you'd want a more secure flow
	userID, err := ms.validatePairingPin(pin)
	if err != nil {
		log.Error().Err(err).Str("pin", pin).Msg("Invalid pairing PIN")
		ms.writeErrorResponse(w, 401, "Invalid PIN")
		return
	}
	
	// Generate client certificate
	cert := ms.generateClientCertificate(uniqueID)
	
	// Store paired client
	client := &PairedClient{
		UniqueID:     uniqueID,
		Certificate:  cert,
		UserID:       userID,
		PairedAt:     time.Now(),
		LastActivity: time.Now(),
	}
	
	ms.pairedClients[cert] = client
	
	log.Info().
		Str("user_id", userID).
		Str("unique_id", uniqueID).
		Str("certificate", cert[:8]+"...").
		Msg("Paired Moonlight client with Helix user")
	
	// Respond with pairing success
	response := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root>
	<status_code>200</status_code>
	<status_message>OK</status_message>
	<certificate>%s</certificate>
	<uniqueid>%s</uniqueid>
</root>`, cert, uniqueID)
	
	w.Write([]byte(response))
}

// validatePairingPin validates a single-use PIN against stored pairing info
func (ms *MoonlightServer) validatePairingPin(pin string) (string, error) {
	pairingInfo, exists := ms.pairingPINs[pin]
	if !exists {
		return "", fmt.Errorf("invalid PIN")
	}
	
	// Check if PIN is expired
	if time.Now().After(pairingInfo.ExpiresAt) {
		delete(ms.pairingPINs, pin)
		return "", fmt.Errorf("PIN expired")
	}
	
	// Check if PIN has already been used
	if pairingInfo.Used {
		delete(ms.pairingPINs, pin)
		return "", fmt.Errorf("PIN already used")
	}
	
	// Mark PIN as used (single-use)
	pairingInfo.Used = true
	
	// Clean up used PIN after a short delay
	go func() {
		time.Sleep(1 * time.Minute)
		delete(ms.pairingPINs, pin)
	}()
	
	return pairingInfo.UserID, nil
}

// GeneratePairingPIN creates a new single-use PIN for a user
func (ms *MoonlightServer) GeneratePairingPIN(userID string) string {
	// Generate 6-digit PIN
	bytes := make([]byte, 3)
	rand.Read(bytes)
	pin := fmt.Sprintf("%06d", int(bytes[0])<<16|int(bytes[1])<<8|int(bytes[2])%1000000)
	
	// Store PIN with 5-minute expiration
	ms.pairingPINs[pin] = &PairingPIN{
		PIN:       pin,
		UserID:    userID,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Used:      false,
	}
	
	log.Info().
		Str("user_id", userID).
		Str("pin", pin).
		Msg("Generated Moonlight pairing PIN")
	
	return pin
}

// generateClientCertificate creates a unique certificate for the client
func (ms *MoonlightServer) generateClientCertificate(uniqueID string) string {
	// Generate random certificate
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// getUserFromContext extracts the user ID from request context
func (ms *MoonlightServer) getUserFromContext(r *http.Request) string {
	if userID, ok := r.Context().Value("moonlight_user_id").(string); ok {
		return userID
	}
	return ""
}

// handleServerInfo responds to Moonlight serverinfo requests
func (ms *MoonlightServer) handleServerInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	
	response := `<?xml version="1.0" encoding="utf-8"?>
<root>
	<hostname>Helix Moonlight Server</hostname>
	<appversion>1.0.0</appversion>
	<GfeVersion>3.20.4.14</GfeVersion>
	<uniqueid>HELIX001</uniqueid>
	<HttpsPort>47984</HttpsPort>
	<ExternalPort>47989</ExternalPort>
	<MaxLumaPixelsHEVC>1869449984</MaxLumaPixelsHEVC>
	<LocalIP>%s</LocalIP>
	<ExternalIP>%s</ExternalIP>
	<PairStatus>1</PairStatus>
	<currentgame>0</currentgame>
	<state>SUNSHINE_SERVER_AVAILABLE</state>
	<StateDescription>Server available</StateDescription>
</root>`

	// Extract host from request or use configured public URL
	host := r.Host
	if host == "" {
		host = ms.publicURL
	}
	
	fmt.Fprintf(w, response, host, host)
	
	log.Debug().
		Str("client_ip", r.RemoteAddr).
		Str("user_agent", r.UserAgent()).
		Msg("Served Moonlight serverinfo")
}

// handleAppList responds to Moonlight applist requests
func (ms *MoonlightServer) handleAppList(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	
	// Get user ID from authenticated context
	userID := ms.getUserFromContext(r)
	if userID == "" {
		ms.writeErrorResponse(w, 401, "Authentication required")
		return
	}
	
	// Refresh apps from active sessions for this user
	ms.refreshAppsFromSessions(userID)
	
	response := `<?xml version="1.0" encoding="utf-8"?>
<root>
	<status_code>200</status_code>
	<status_message>OK</status_message>`
	
	for _, app := range ms.apps {
		supportHDR := "0"
		if app.SupportHDR {
			supportHDR = "1"
		}
		
		iconPNG := app.IconPNGPath
		if iconPNG == "" {
			iconPNG = "https://via.placeholder.com/128x128.png?text=" + app.Title
		}
		
		response += fmt.Sprintf(`
	<App>
		<AppTitle>%s</AppTitle>
		<ID>%d</ID>
		<IsRunning>0</IsRunning>
		<IconPNG>%s</IconPNG>
		<SupportHDR>%s</SupportHDR>
	</App>`, app.Title, app.ID, iconPNG, supportHDR)
	}
	
	response += "\n</root>"
	
	w.Write([]byte(response))
	
	log.Debug().
		Str("client_ip", r.RemoteAddr).
		Int("app_count", len(ms.apps)).
		Msg("Served Moonlight applist")
}

// handleLaunch responds to Moonlight launch requests
func (ms *MoonlightServer) handleLaunch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	
	// Parse app ID from query parameters
	appIDStr := r.URL.Query().Get("appid")
	if appIDStr == "" {
		ms.writeErrorResponse(w, 400, "Missing appid parameter")
		return
	}
	
	appID, err := strconv.ParseUint(appIDStr, 10, 64)
	if err != nil {
		ms.writeErrorResponse(w, 400, "Invalid appid parameter")
		return
	}
	
	// Find the app
	app, exists := ms.apps[appID]
	if !exists {
		ms.writeErrorResponse(w, 404, "App not found")
		return
	}
	
	// Generate session ID (timestamp-based for uniqueness)
	sessionID := uint64(time.Now().UnixNano())
	
	// Get client IP
	clientIP := strings.Split(r.RemoteAddr, ":")[0]
	
	// Create secret payload for RTP identification (random bytes)
	var secretPayload [16]byte
	for i := range secretPayload {
		secretPayload[i] = byte(sessionID + uint64(i)) // Simple deterministic generation
	}
	
	// Register session with proxy
	err = ms.proxy.RegisterSession(sessionID, appID, app.HelixSessionID, app.RunnerID, clientIP, secretPayload)
	if err != nil {
		log.Error().Err(err).
			Uint64("session_id", sessionID).
			Uint64("app_id", appID).
			Str("runner_id", app.RunnerID).
			Msg("Failed to register Moonlight session")
		ms.writeErrorResponse(w, 500, "Failed to start session")
		return
	}
	
	// Build RTSP URL for session negotiation
	rtspURL := fmt.Sprintf("rtsp://%s:48010", r.Host)
	
	response := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root>
	<status_code>200</status_code>
	<status_message>OK</status_message>
	<gfeversion>3.20.4.14</gfeversion>
	<gamesession>%d</gamesession>
	<sessionUrl0>%s</sessionUrl0>
</root>`, sessionID, rtspURL)
	
	w.Write([]byte(response))
	
	log.Info().
		Uint64("session_id", sessionID).
		Uint64("app_id", appID).
		Str("app_title", app.Title).
		Str("runner_id", app.RunnerID).
		Str("client_ip", clientIP).
		Msg("Launched Moonlight session")
}

// handleResume responds to Moonlight resume requests
func (ms *MoonlightServer) handleResume(w http.ResponseWriter, r *http.Request) {
	// For now, treat resume the same as launch
	ms.handleLaunch(w, r)
}

// handleCancel responds to Moonlight cancel requests
func (ms *MoonlightServer) handleCancel(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/xml")
	
	// Parse session ID if provided
	sessionIDStr := r.URL.Query().Get("gamesession")
	if sessionIDStr != "" {
		if sessionID, err := strconv.ParseUint(sessionIDStr, 10, 64); err == nil {
			ms.proxy.UnregisterSession(sessionID)
			log.Info().Uint64("session_id", sessionID).Msg("Cancelled Moonlight session")
		}
	}
	
	response := `<?xml version="1.0" encoding="utf-8"?>
<root>
	<status_code>200</status_code>
	<status_message>OK</status_message>
</root>`
	
	w.Write([]byte(response))
}

// handleQuit responds to Moonlight quit requests
func (ms *MoonlightServer) handleQuit(w http.ResponseWriter, r *http.Request) {
	// Treat quit the same as cancel
	ms.handleCancel(w, r)
}

// handleRTSP proxies RTSP traffic to the appropriate backend
func (ms *MoonlightServer) handleRTSP(w http.ResponseWriter, r *http.Request) {
	// For now, return a simple response
	// In a full implementation, this would proxy RTSP traffic to Wolf on the backend
	w.Header().Set("Content-Type", "application/sdp")
	w.WriteHeader(http.StatusOK)
	
	response := `v=0
o=- 0 0 IN IP4 127.0.0.1
s=HELIX Moonlight Stream
c=IN IP4 127.0.0.1
t=0 0
m=video 48100 RTP/AVP 96
a=rtpmap:96 H264/90000
m=audio 48200 RTP/AVP 97
a=rtpmap:97 OPUS/48000/2`
	
	w.Write([]byte(response))
	
	log.Debug().
		Str("method", r.Method).
		Str("client_ip", r.RemoteAddr).
		Msg("Handled RTSP request")
}

// refreshAppsFromSessions updates the app list based on user's accessible sessions
func (ms *MoonlightServer) refreshAppsFromSessions(userID string) {
	ctx := context.Background()
	
	// Clear existing apps for this refresh
	ms.apps = make(map[uint64]*MoonlightApp)
	
	// Get user to check permissions
	user, err := ms.store.GetUser(ctx, &store.GetUserQuery{ID: userID})
	if err != nil {
		log.Error().Err(err).Str("user_id", userID).Msg("Failed to get user for app list")
		return
	}
	
	// Log user info for session access
	log.Debug().Str("user_id", user.ID).Msg("Refreshing apps for user")
	
	appID := uint64(1)
	
	// Add default desktop app for accessible agent runners
	// In practice, you'd query for agent runners this user has access to
	ms.apps[appID] = &MoonlightApp{
		ID:         appID,
		Title:      "Helix Desktop",
		RunnerID:   "default-agent-runner",
		SupportHDR: false,
	}
	appID++
	
	// Get active work sessions for this user
	// TODO: Implement actual session filtering based on user permissions
	// Example implementation:
	/*
	sessions, err := ms.store.GetActiveWorkSessionsForUser(ctx, userID)
	if err == nil {
		for _, session := range sessions {
			// Check if user has access to this session
			if ms.userCanAccessSession(user, session) {
				ms.apps[appID] = &MoonlightApp{
					ID:             appID,
					Title:          fmt.Sprintf("Session: %s", session.Name),
					HelixSessionID: session.HelixSessionID,
					RunnerID:       session.RunnerID,
					SupportHDR:     false,
				}
				appID++
			}
		}
	}
	
	// Get accessible external agent runners
	agentRunners, err := ms.store.GetAccessibleAgentRunnersForUser(ctx, userID)
	if err == nil {
		for _, agentRunner := range agentRunners {
			if ms.userCanAccessAgentRunner(user, agentRunner) {
				ms.apps[appID] = &MoonlightApp{
					ID:         appID,
					Title:      fmt.Sprintf("Agent Runner: %s", agentRunner.Name),
					RunnerID:   agentRunner.ID,
					SupportHDR: false,
				}
				appID++
			}
		}
	}
	*/
	
	log.Debug().
		Str("user_id", userID).
		Int("app_count", len(ms.apps)).
		Msg("Refreshed Moonlight apps for user")
}

// userCanAccessSession checks if user has permission to access a session
func (ms *MoonlightServer) userCanAccessSession(user *types.User, session interface{}) bool {
	// Implement your RBAC logic here
	// Examples:
	// - User owns the session
	// - User is in the same organization
	// - User has admin privileges
	// - Session is shared with user
	return true // Simplified for demo
}

// userCanAccessAgentRunner checks if user has permission to access an agent runner
func (ms *MoonlightServer) userCanAccessAgentRunner(user *types.User, agentRunner interface{}) bool {
	// Implement your RBAC logic here
	// Examples:
	// - Agent runner is in user's organization
	// - User has agent runner access role
	// - User is admin
	return true // Simplified for demo
}

// Admin endpoints for managing apps

// handleListApps returns the current app configuration
func (ms *MoonlightServer) handleListApps(w http.ResponseWriter, r *http.Request) {
	ms.refreshAppsFromSessions("")
	
	apps := make([]*MoonlightApp, 0, len(ms.apps))
	for _, app := range ms.apps {
		apps = append(apps, app)
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apps)
}

// handleCreateApp creates a new app configuration
func (ms *MoonlightServer) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	var app MoonlightApp
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	// Auto-assign ID
	app.ID = uint64(len(ms.apps) + 1)
	ms.apps[app.ID] = &app
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&app)
	
	log.Info().
		Uint64("app_id", app.ID).
		Str("title", app.Title).
		Str("runner_id", app.RunnerID).
		Msg("Created Moonlight app")
}

// handleUpdateApp updates an existing app configuration
func (ms *MoonlightServer) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appIDStr := vars["app_id"]
	
	appID, err := strconv.ParseUint(appIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid app ID", http.StatusBadRequest)
		return
	}
	
	var app MoonlightApp
	if err := json.NewDecoder(r.Body).Decode(&app); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}
	
	app.ID = appID
	ms.apps[appID] = &app
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(&app)
}

// handleDeleteApp removes an app configuration
func (ms *MoonlightServer) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	appIDStr := vars["app_id"]
	
	appID, err := strconv.ParseUint(appIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid app ID", http.StatusBadRequest)
		return
	}
	
	delete(ms.apps, appID)
	w.WriteHeader(http.StatusNoContent)
}

// handleRefreshApps manually refreshes the app list
func (ms *MoonlightServer) handleRefreshApps(w http.ResponseWriter, r *http.Request) {
	ms.refreshAppsFromSessions("")
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "ok",
		"app_count": len(ms.apps),
	})
}

// writeErrorResponse writes an XML error response
func (ms *MoonlightServer) writeErrorResponse(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(code)
	
	response := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<root>
	<status_code>%d</status_code>
	<status_message>%s</status_message>
</root>`, code, message)
	
	w.Write([]byte(response))
}