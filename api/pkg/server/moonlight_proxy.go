package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// proxyToMoonlightWeb reverse proxies requests to the moonlight-web service
// Validates Helix user authentication and injects moonlight-web credentials
func (apiServer *HelixAPIServer) proxyToMoonlightWeb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	log.Info().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("query", r.URL.RawQuery).
		Str("remote_addr", r.RemoteAddr).
		Str("upgrade_header", r.Header.Get("Upgrade")).
		Str("connection_header", r.Header.Get("Connection")).
		Bool("is_websocket", r.Header.Get("Upgrade") == "websocket").
		Msg("🌐 Moonlight proxy request received")

	// Extract user context (added by auth middleware)
	user := getRequestUser(r)

	// RBAC: User must be authenticated to access moonlight streaming
	if user == nil {
		log.Warn().
			Str("path", r.URL.Path).
			Msg("❌ Unauthenticated request to moonlight proxy")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("path", r.URL.Path).
		Msg("✅ User authenticated for moonlight proxy")

	// Extract session ID from moonlight streaming request
	// Format: "agent-ses_xxx" or "agent-req_xxx" in session_id query param
	sessionID := apiServer.extractHelixSessionFromMoonlightRequest(r)

	// If we have a session ID, verify user has access
	if sessionID != "" {
		// Check access to session (checks owner, access grants, org membership)
		if !apiServer.canUserStreamSession(ctx, user, sessionID) {
			log.Warn().
				Str("session_id", sessionID).
				Str("user_id", user.ID).
				Msg("Streaming access denied")
			http.Error(w, "Access denied - you don't have permission to stream this session", http.StatusForbidden)
			return
		}

		log.Debug().
			Str("session_id", sessionID).
			Str("user_id", user.ID).
			Msg("Streaming access granted")
	}

	// Moonlight Web service URL (from docker-compose network)
	moonlightWebURL := "http://moonlight-web:8080"

	// Handle WebSocket upgrade separately (reverse proxy can't handle this)
	if r.Header.Get("Upgrade") == "websocket" {
		apiServer.proxyWebSocket(w, r, moonlightWebURL, user)
		return
	}

	// Handle regular HTTP requests with reverse proxy
	target, err := url.Parse(moonlightWebURL)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse moonlight-web URL")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// Customize proxy director to inject moonlight credentials
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)

		// Inject moonlight-web credentials for backend authentication
		moonlightCreds := apiServer.getMoonlightCredentials()
		req.Header.Set("Authorization", "Bearer "+moonlightCreds)

		log.Debug().
			Str("user_id", user.ID).
			Str("original_path", r.URL.Path).
			Str("proxy_path", req.URL.Path).
			Msg("Proxying HTTP request to moonlight-web")
	}

	// Modify the request to remove /moonlight prefix
	r.URL.Path = strings.TrimPrefix(r.URL.Path, "/moonlight")
	if r.URL.Path == "" {
		r.URL.Path = "/"
	}

	// Proxy the request
	proxy.ServeHTTP(w, r)
}

// extractHelixSessionFromMoonlightRequest extracts Helix session ID from moonlight request
// Moonlight uses session_id like "agent-ses_xxx" or "agent-req_xxx"
func (apiServer *HelixAPIServer) extractHelixSessionFromMoonlightRequest(r *http.Request) string {
	// Check query parameter (used in WebSocket stream requests)
	moonlightSessionID := r.URL.Query().Get("session_id")
	if moonlightSessionID == "" {
		// Check POST body for launch requests
		// For now, only WebSocket streams have session IDs
		return ""
	}

	// Extract Helix session ID from moonlight format
	// Format: "agent-ses_01k83m6bws1r77ez98w8kp83xb" → "ses_01k83m6bws1r77ez98w8kp83xb"
	// Format: "agent-req_01k83m6bws1r77ez98w8kp83xb" → "req_01k83m6bws1r77ez98w8kp83xb"
	if strings.HasPrefix(moonlightSessionID, "agent-") {
		return strings.TrimPrefix(moonlightSessionID, "agent-")
	}

	// Also handle kickoff sessions (agent-ses_xxx-kickoff)
	if strings.Contains(moonlightSessionID, "-kickoff") {
		// Extract: "agent-ses_xxx-kickoff" → "ses_xxx"
		sessionID := strings.TrimPrefix(moonlightSessionID, "agent-")
		sessionID = strings.TrimSuffix(sessionID, "-kickoff")
		return sessionID
	}

	return ""
}

// getMoonlightCredentials returns the moonlight-web authentication credentials
// SECURITY: Read directly from env, do NOT expose via config endpoint
func (apiServer *HelixAPIServer) getMoonlightCredentials() string {
	// Get from environment (set during install.sh with random password)
	// NEVER add this to config structs - would leak via /api/v1/config endpoint
	creds := os.Getenv("MOONLIGHT_CREDENTIALS")
	if creds == "" {
		// Fallback to default for development
		creds = "helix"
	}
	return creds
}

// proxyWebSocket handles WebSocket upgrade and proxies to moonlight-web
func (apiServer *HelixAPIServer) proxyWebSocket(w http.ResponseWriter, r *http.Request, moonlightWebURL string, user *types.User) {
	// Remove /moonlight prefix for backend
	// Note: moonlight-web WebSocket is at /api/host/stream, not just /host/stream
	backendPath := strings.TrimPrefix(r.URL.Path, "/moonlight")
	if backendPath == "" {
		backendPath = "/"
	}

	// If the path doesn't start with /api, prepend it (moonlight-web WebSocket needs /api prefix)
	if !strings.HasPrefix(backendPath, "/api/") {
		backendPath = "/api" + backendPath
	}

	// Build backend WebSocket URL (ws:// for http://, wss:// for https://)
	backendURL := fmt.Sprintf("ws://moonlight-web:8080%s", backendPath)
	if r.URL.RawQuery != "" {
		backendURL += "?" + r.URL.RawQuery
	}

	log.Info().
		Str("user_id", user.ID).
		Str("backend_url", backendURL).
		Msg("Upgrading WebSocket connection for moonlight streaming")

	// Upgrade client connection
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (already auth'd)
		},
	}

	clientConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade client WebSocket connection")
		return
	}
	defer clientConn.Close()

	// Connect to moonlight-web backend with auth
	backendHeaders := http.Header{}
	moonlightCreds := apiServer.getMoonlightCredentials()
	backendHeaders.Set("Authorization", "Bearer "+moonlightCreds)

	backendConn, resp, err := websocket.DefaultDialer.Dial(backendURL, backendHeaders)
	if err != nil {
		log.Error().
			Err(err).
			Str("backend_url", backendURL).
			Int("status", func() int {
				if resp != nil {
					return resp.StatusCode
				}
				return 0
			}()).
			Msg("Failed to connect to moonlight-web WebSocket")
		return
	}
	defer backendConn.Close()
	if resp != nil {
		defer resp.Body.Close()
	}

	log.Info().
		Str("user_id", user.ID).
		Str("backend_url", backendURL).
		Msg("✅ WebSocket proxy established - streaming active")

	// Proxy messages bidirectionally
	errCh := make(chan error, 2)

	// Client -> Backend (with credential translation)
	go func() {
		moonlightCreds := apiServer.getMoonlightCredentials()

		for {
			messageType, message, err := clientConn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("client read error: %w", err)
				return
			}

			// For text messages, check if it's AuthenticateAndInit and replace credentials
			var transformedMessage []byte = message
			if messageType == websocket.TextMessage {
				var msg map[string]interface{}
				if err := json.Unmarshal(message, &msg); err == nil {
					// Check if this is AuthenticateAndInit message
					if authInitRaw, exists := msg["AuthenticateAndInit"]; exists {
						if authInit, ok := authInitRaw.(map[string]interface{}); ok {
							// Replace Helix JWT with moonlight credentials
							log.Info().
								Str("user_id", user.ID).
								Str("old_creds_prefix", func() string {
									if creds, ok := authInit["credentials"].(string); ok && len(creds) > 20 {
										return creds[:20] + "..."
									}
									return "unknown"
								}()).
								Str("new_creds", moonlightCreds).
								Msg("🔄 Replacing client credentials with moonlight credentials")
							authInit["credentials"] = moonlightCreds
							msg["AuthenticateAndInit"] = authInit

							var marshalErr error
							transformedMessage, marshalErr = json.Marshal(msg)
							if marshalErr != nil {
								log.Error().Err(marshalErr).Msg("Failed to marshal transformed message")
								transformedMessage = message // Fallback to original
							} else {
								log.Debug().
									Int("original_len", len(message)).
									Int("transformed_len", len(transformedMessage)).
									Msg("✅ Credentials replaced successfully")
							}
						} else {
							log.Warn().
								Str("type", fmt.Sprintf("%T", authInitRaw)).
								Msg("⚠️ AuthenticateAndInit is not a map")
						}
					}
				} else {
					log.Debug().Err(err).Msg("Message is not JSON, forwarding as-is")
				}
			}

			log.Debug().
				Str("user_id", user.ID).
				Int("message_type", messageType).
				Int("message_len", len(transformedMessage)).
				Bool("credentials_replaced", len(transformedMessage) != len(message)).
				Msg("📨 Client -> Backend message")

			if err := backendConn.WriteMessage(messageType, transformedMessage); err != nil {
				errCh <- fmt.Errorf("backend write error: %w", err)
				return
			}
		}
	}()

	// Backend -> Client
	go func() {
		for {
			messageType, message, err := backendConn.ReadMessage()
			if err != nil {
				errCh <- fmt.Errorf("backend read error: %w", err)
				return
			}
			log.Debug().
				Str("user_id", user.ID).
				Int("message_type", messageType).
				Int("message_len", len(message)).
				Msg("📩 Backend -> Client message")
			if err := clientConn.WriteMessage(messageType, message); err != nil {
				errCh <- fmt.Errorf("client write error: %w", err)
				return
			}
		}
	}()

	// Wait for error or disconnect
	err = <-errCh
	log.Info().
		Err(err).
		Str("user_id", user.ID).
		Msg("WebSocket proxy connection closed")
}

// canUserStreamSession checks if user has permission to stream a session
// Checks: owner, admin, access grants (TODO), org membership (TODO)
func (apiServer *HelixAPIServer) canUserStreamSession(ctx context.Context, user *types.User, sessionID string) bool {
	// Get the session
	session, err := apiServer.Store.GetSession(ctx, sessionID)
	if err != nil {
		log.Warn().
			Err(err).
			Str("session_id", sessionID).
			Str("user_id", user.ID).
			Msg("Session not found for streaming access check")
		return false
	}

	// Check 1: User is owner
	if session.Owner == user.ID {
		return true
	}

	// Check 2: User is admin
	if isAdmin(user) {
		return true
	}

	// Check 3: TODO - Check AccessGrant for this session
	// When AccessGrant system is extended to sessions, check:
	// grants, err := apiServer.Store.ListAccessGrants(ctx, &store.ListAccessGrantsQuery{
	//     ResourceID: sessionID,
	//     UserID: user.ID,
	// })
	// if len(grants) > 0 { return true }

	// Check 4: TODO - For external agent sessions, check parent resource access
	// If session belongs to an app/PDE/spectask, check access to that resource
	// This allows users to stream agent sessions for resources they can access

	// No access found
	return false
}

// getMoonlightStatus returns the internal state of moonlight-web for observability
// Exposes moonlight-web's internal session state, client certificates, and streamer process health
// @Summary Get moonlight-web internal state
// @Description Returns active streaming sessions, client certificates, and WebSocket connection state from moonlight-web
// @Tags Moonlight
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/moonlight/status [get]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) getMoonlightStatus(res http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	user := getRequestUser(req)

	if user == nil {
		http.Error(res, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Fetch moonlight-web admin status (includes all clients + sessions)
	moonlightURL := "http://moonlight-web:8080/api/admin/status"
	httpReq, err := http.NewRequestWithContext(ctx, "GET", moonlightURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request to moonlight-web")
		http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Add moonlight-web credentials
	moonlightCreds := apiServer.getMoonlightCredentials()
	httpReq.Header.Set("Authorization", "Bearer "+moonlightCreds)

	client := &http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch moonlight-web status")
		http.Error(res, "Failed to fetch moonlight-web status", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(resp.StatusCode)
	_, _ = res.Write([]byte{}) // Will be filled by copying response body

	// Copy response body
	buf := make([]byte, 32*1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			res.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}
}
