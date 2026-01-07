package server

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Minimum time between streaming connections for the same session (prevents Wolf deadlock)
const streamingRateLimitDuration = 3 * time.Second

// proxyToMoonlightWeb reverse proxies requests to the moonlight-web service
// Validates Helix user authentication and injects moonlight-web credentials
func (apiServer *HelixAPIServer) proxyToMoonlightWeb(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	log.Error().
		Str("method", r.Method).
		Str("path", r.URL.Path).
		Str("query", r.URL.RawQuery).
		Str("remote_addr", r.RemoteAddr).
		Str("upgrade_header", r.Header.Get("Upgrade")).
		Str("connection_header", r.Header.Get("Connection")).
		Bool("is_websocket", r.Header.Get("Upgrade") == "websocket").
		Msg("🌐🌐🌐 MOONLIGHT PROXY REQUEST RECEIVED 🌐🌐🌐")

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

	// Determine which Moonlight Web instance to route to based on session's Wolf instance
	var moonlightRunnerID string

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

		// Get session to determine Wolf instance
		session, err := apiServer.Store.GetSession(ctx, sessionID)
		if err != nil {
			log.Error().
				Err(err).
				Str("session_id", sessionID).
				Msg("Failed to get session for Moonlight routing")
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}

		// Route to the Wolf instance running this session
		// Format: moonlight-{wolfInstanceID}
		if session.WolfInstanceID != "" {
			moonlightRunnerID = "moonlight-" + session.WolfInstanceID
			log.Debug().
				Str("session_id", sessionID).
				Str("wolf_instance_id", session.WolfInstanceID).
				Str("moonlight_runner_id", moonlightRunnerID).
				Msg("Routing to Wolf instance's Moonlight Web")
		}
	}

	// Fallback to environment variable or dev default
	if moonlightRunnerID == "" {
		moonlightRunnerID = os.Getenv("MOONLIGHT_RUNNER_ID")
		if moonlightRunnerID == "" {
			moonlightRunnerID = "moonlight-dev" // Default for dev mode
		}
	}

	log.Debug().
		Str("runner_id", moonlightRunnerID).
		Str("path", r.URL.Path).
		Msg("Proxying to Moonlight Web via RevDial")

	// Dial Moonlight Web via RevDial
	conn, err := apiServer.connman.Dial(ctx, moonlightRunnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", moonlightRunnerID).
			Msg("Failed to dial Moonlight Web via RevDial")
		http.Error(w, "Failed to connect to Moonlight Web - check that sandbox is running", http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	log.Debug().
		Str("runner_id", moonlightRunnerID).
		Msg("Connected to Moonlight Web via RevDial")

	// Handle WebSocket upgrade (for WebRTC signaling and WebSocket streaming)
	if r.Header.Get("Upgrade") == "websocket" {
		// Rate limit streaming connections to prevent Wolf deadlock from rapid reconnects
		// This protects against both accidental (frontend bugs) and malicious (DOS) rapid connections
		if sessionID != "" {
			if !apiServer.checkStreamingRateLimit(sessionID) {
				log.Warn().
					Str("session_id", sessionID).
					Str("user_id", user.ID).
					Msg("Streaming connection rate limited (too frequent reconnects)")
				http.Error(w, "Too many connection attempts - please wait a few seconds", http.StatusTooManyRequests)
				return
			}
		}
		apiServer.proxyWebSocketViaRevDial(w, r, conn, user)
		return
	}

	// Handle regular HTTP requests via RevDial tunnel
	// Strip /moonlight prefix and add /api prefix for Moonlight Web
	// Frontend sends: /moonlight/api/sessions
	// Moonlight Web expects: /api/sessions
	backendPath := strings.TrimPrefix(r.URL.Path, "/moonlight")
	if backendPath == "" {
		backendPath = "/"
	}
	if !strings.HasPrefix(backendPath, "/api/") {
		backendPath = "/api" + backendPath
	}

	log.Debug().
		Str("original_path", r.URL.Path).
		Str("backend_path", backendPath).
		Msg("Forwarding HTTP request to Moonlight Web via RevDial")

	// Clone request with modified path
	backendReq := r.Clone(ctx)
	backendReq.URL.Path = backendPath
	backendReq.RequestURI = backendPath
	if r.URL.RawQuery != "" {
		backendReq.RequestURI += "?" + r.URL.RawQuery
	}

	// Add moonlight-web authentication credentials
	// This is required for ALL requests to moonlight-web (HTTP and WebSocket)
	moonlightCreds := apiServer.getMoonlightCredentials()
	backendReq.Header.Set("Authorization", "Bearer "+moonlightCreds)

	log.Debug().
		Str("path", backendPath).
		Bool("has_auth", backendReq.Header.Get("Authorization") != "").
		Msg("Forwarding HTTP request to Moonlight Web with credentials")

	// Write HTTP request to RevDial connection
	if err := backendReq.Write(conn); err != nil {
		log.Error().Err(err).Msg("Failed to write request to RevDial connection")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Read HTTP response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), r)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response from RevDial connection")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Copy response status and body
	w.WriteHeader(resp.StatusCode)

	// Check if this is an SSE response - needs special handling to prevent buffering
	if resp.Header.Get("Content-Type") == "text/event-stream" {
		log.Debug().Msg("Proxying SSE stream - using flush-friendly copy")

		// Get flusher for streaming response
		flusher, ok := w.(http.Flusher)
		if !ok {
			log.Error().Msg("ResponseWriter does not support Flusher - SSE will be buffered")
		}

		// Copy with periodic flushing for SSE
		// DEBUG: Use larger buffer to avoid fragmenting SSE events
		buf := make([]byte, 256*1024) // 256KB buffer for large video frames
		totalBytes := 0
		chunkCount := 0
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				totalBytes += n
				chunkCount++
				// Log every 30 chunks or first 5 to debug truncation
				if chunkCount <= 5 || chunkCount%30 == 0 {
					log.Debug().Int("chunk", chunkCount).Int("bytes", n).Int("totalBytes", totalBytes).Msg("SSE chunk read")
				}
				_, writeErr := w.Write(buf[:n])
				if writeErr != nil {
					log.Error().Err(writeErr).Msg("Failed to write SSE data")
					return
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr == io.EOF {
				log.Debug().Int("totalBytes", totalBytes).Int("chunks", chunkCount).Msg("SSE stream ended")
				break
			}
			if readErr != nil {
				log.Error().Err(readErr).Msg("Failed to read SSE data")
				return
			}
		}
	} else {
		// Regular HTTP response
		if _, err := io.Copy(w, resp.Body); err != nil {
			log.Error().Err(err).Msg("Failed to copy response body")
			return
		}
	}
}

// proxyWebSocketViaRevDial proxies WebSocket connections through RevDial tunnel
func (apiServer *HelixAPIServer) proxyWebSocketViaRevDial(w http.ResponseWriter, r *http.Request, revdialConn net.Conn, user *types.User) {
	// Upgrade client connection to WebSocket
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins (RBAC already enforced)
		},
	}

	clientWS, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to upgrade client WebSocket")
		return
	}
	defer clientWS.Close()

	// Strip /moonlight prefix and add /api prefix for Moonlight Web
	// Frontend sends: /moonlight/host/stream
	// Moonlight Web expects: /api/host/stream
	backendPath := strings.TrimPrefix(r.URL.Path, "/moonlight")
	if backendPath == "" {
		backendPath = "/"
	}
	if !strings.HasPrefix(backendPath, "/api/") {
		backendPath = "/api" + backendPath
	}

	log.Debug().
		Str("user_id", user.ID).
		Str("original_path", r.URL.Path).
		Str("backend_path", backendPath).
		Msg("Client WebSocket upgraded, sending request to Moonlight Web via RevDial")

	// Clone the request with modified path
	backendReq := r.Clone(r.Context())
	backendReq.URL.Path = backendPath
	backendReq.RequestURI = backendPath
	if r.URL.RawQuery != "" {
		backendReq.RequestURI += "?" + r.URL.RawQuery
	}

	// Instead of manually handling WebSocket frames over raw TCP,
	// create a proper WebSocket client connection via RevDial using a custom dialer
	moonlightCreds := apiServer.getMoonlightCredentials()

	// Create custom dialer that uses our existing RevDial connection
	backendWSURL := fmt.Sprintf("ws://moonlight-web%s", backendPath)
	if r.URL.RawQuery != "" {
		backendWSURL += "?" + r.URL.RawQuery
	}

	log.Debug().
		Str("backend_url", backendWSURL).
		Msg("Creating WebSocket connection to Moonlight Web via RevDial")

	// Create WebSocket connection using the RevDial connection as transport
	backendHeaders := http.Header{}
	backendHeaders.Set("Authorization", "Bearer "+moonlightCreds)

	dialer := websocket.Dialer{
		NetDial: func(network, addr string) (net.Conn, error) {
			// Return our existing RevDial connection instead of creating a new one
			return revdialConn, nil
		},
	}

	backendWS, resp, err := dialer.Dial(backendWSURL, backendHeaders)
	if err != nil {
		log.Error().
			Err(err).
			Str("backend_url", backendWSURL).
			Int("status", func() int {
				if resp != nil {
					return resp.StatusCode
				}
				return 0
			}()).
			Msg("Failed to establish WebSocket to Moonlight Web via RevDial")
		return
	}
	defer backendWS.Close()
	if resp != nil {
		defer resp.Body.Close()
	}

	log.Debug().Msg("WebSocket connection established to Moonlight Web, starting bidirectional proxy")

	errChan := make(chan error, 3) // 3 for client→backend, backend→client, and ping

	// Mutex for thread-safe writes to client WebSocket (ping and backend→client can race)
	var clientMu sync.Mutex

	// Start server-initiated ping goroutine to keep client connection alive through proxies/firewalls
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			clientMu.Lock()
			err := clientWS.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
			clientMu.Unlock()
			if err != nil {
				log.Debug().Err(err).Msg("Moonlight WebSocket proxy ping failed, connection closing")
				errChan <- nil // Signal connection closed
				return
			}
		}
	}()

	// Client → Moonlight Web (with credential replacement on first message)
	go func() {
		moonlightCreds := apiServer.getMoonlightCredentials()
		firstMessage := true

		for {
			messageType, message, err := clientWS.ReadMessage()
			if err != nil {
				if err != io.EOF && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- fmt.Errorf("client read error: %w", err)
				} else {
					errChan <- nil
				}
				return
			}

			// Log binary input messages for debugging
			if messageType == websocket.BinaryMessage && len(message) > 0 {
				log.Debug().Int("msg_type", int(message[0])).Int("len", len(message)).Msg("🔤 Forwarding binary message to Moonlight Web")
			}

			// Replace Helix JWT with Moonlight credentials in first message (AuthenticateAndInit)
			transformedMessage := message
			if firstMessage && messageType == websocket.TextMessage {
				var msg map[string]interface{}
				if err := json.Unmarshal(message, &msg); err == nil {
					if authInitRaw, exists := msg["AuthenticateAndInit"]; exists {
						if authInit, ok := authInitRaw.(map[string]interface{}); ok {
							log.Debug().
								Str("user_id", user.ID).
								Str("old_creds_prefix", func() string {
									if creds, ok := authInit["credentials"].(string); ok && len(creds) > 20 {
										return creds[:20] + "..."
									}
									return "unknown"
								}()).
								Str("new_creds", moonlightCreds).
								Msg("🔄 Replacing Helix JWT with Moonlight credentials")

							authInit["credentials"] = moonlightCreds
							msg["AuthenticateAndInit"] = authInit

							if transformedBytes, err := json.Marshal(msg); err == nil {
								transformedMessage = transformedBytes
								log.Debug().Msg("✅ Credentials replaced successfully")
							} else {
								log.Error().Err(err).Msg("Failed to marshal transformed message, using original")
							}
						}
					}
				}
				firstMessage = false
			}

			// Forward to Moonlight Web
			if err := backendWS.WriteMessage(messageType, transformedMessage); err != nil {
				errChan <- fmt.Errorf("backend write error: %w", err)
				return
			}
		}
	}()

	// Moonlight Web → Client
	go func() {
		for {
			messageType, message, err := backendWS.ReadMessage()
			if err != nil {
				if err != io.EOF && !websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
					errChan <- fmt.Errorf("backend read error: %w", err)
				} else {
					errChan <- nil
				}
				return
			}

			// Forward to client (mutex protects against concurrent ping writes)
			clientMu.Lock()
			writeErr := clientWS.WriteMessage(messageType, message)
			clientMu.Unlock()
			if writeErr != nil {
				errChan <- fmt.Errorf("client write error: %w", writeErr)
				return
			}
		}
	}()

	// Wait for either direction to close
	err = <-errChan
	if err != nil {
		log.Debug().Err(err).Msg("WebSocket proxy error")
	}

	log.Debug().Msg("WebSocket proxy connection closed")
}

// extractHelixSessionFromMoonlightRequest extracts Helix session ID from moonlight request
// Moonlight uses session_id like "agent-ses_xxx" or "agent-req_xxx" with optional suffixes
func (apiServer *HelixAPIServer) extractHelixSessionFromMoonlightRequest(r *http.Request) string {
	// Check query parameter (used in WebSocket stream requests)
	moonlightSessionID := r.URL.Query().Get("session_id")
	if moonlightSessionID == "" {
		// Check POST body for launch requests
		// For now, only WebSocket streams have session IDs
		return ""
	}

	// Extract Helix session ID from moonlight format
	// Supported formats (with optional suffixes for multi-tab support):
	// - "agent-ses_01k83m6bws1r77ez98w8kp83xb" → "ses_01k83m6bws1r77ez98w8kp83xb"
	// - "agent-ses_xxx-uuid" → "ses_xxx"
	// - "agent-ses_xxx-kickoff" → "ses_xxx"
	// - "agent-req_xxx-uuid" → "req_xxx"

	// Strip "agent-" prefix if present
	sessionID := strings.TrimPrefix(moonlightSessionID, "agent-")

	// Find the Helix session ID pattern (ses_xxx or req_xxx)
	// The ID is the portion up to the next hyphen after the underscore
	if strings.HasPrefix(sessionID, "ses_") || strings.HasPrefix(sessionID, "req_") {
		// Find the end of the ID (26 chars after ses_/req_)
		// Format: ses_ + 26 chars = 30 total, req_ + 26 chars = 30 total
		prefix := sessionID[:4] // "ses_" or "req_"
		rest := sessionID[4:]

		// Take up to the next hyphen or end of string
		if idx := strings.Index(rest, "-"); idx > 0 {
			return prefix + rest[:idx]
		}
		// No hyphen found, return as-is (simple format)
		return sessionID
	}

	return ""
}

// OLD IMPLEMENTATION - DELETED
// This was the Docker-in-Docker version that connected directly to moonlight-web:8080
// Now we use RevDial proxy above (proxyToMoonlightWeb + proxyWebSocketViaRevDial)
//
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

// proxyWebSocket_OLD - UNUSED (kept for reference, uses Docker-in-Docker architecture)
func (apiServer *HelixAPIServer) proxyWebSocket_OLD(w http.ResponseWriter, r *http.Request, moonlightWebURL string, user *types.User) {
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
	errCh := make(chan error, 3) // 3 for client→backend, backend→client, and ping

	// Mutex for thread-safe writes to client WebSocket (ping and backend→client can race)
	var clientMu sync.Mutex

	// Start server-initiated ping goroutine to keep client connection alive through proxies/firewalls
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			<-ticker.C
			clientMu.Lock()
			err := clientConn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second))
			clientMu.Unlock()
			if err != nil {
				log.Debug().Err(err).Msg("Moonlight WebSocket proxy ping failed, connection closing")
				errCh <- nil // Signal connection closed
				return
			}
		}
	}()

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
			// Mutex protects against concurrent ping writes
			clientMu.Lock()
			writeErr := clientConn.WriteMessage(messageType, message)
			clientMu.Unlock()
			if writeErr != nil {
				errCh <- fmt.Errorf("client write error: %w", writeErr)
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
// @Param wolf_instance_id query string true "Wolf instance ID to query"
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

	// Get wolf_instance_id from query params - required for RevDial routing
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Build RevDial runner ID - moonlight-web registers as "moonlight-{wolfInstanceID}"
	moonlightRunnerID := "moonlight-" + wolfInstanceID

	log.Debug().
		Str("wolf_instance_id", wolfInstanceID).
		Str("runner_id", moonlightRunnerID).
		Msg("Fetching moonlight-web status via RevDial")

	// Dial Moonlight Web via RevDial
	conn, err := apiServer.connman.Dial(ctx, moonlightRunnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", moonlightRunnerID).
			Msg("Failed to dial Moonlight Web via RevDial for status")
		http.Error(res, "Failed to connect to Moonlight Web - sandbox may not be running", http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	// Create HTTP request for admin status endpoint
	httpReq, err := http.NewRequestWithContext(ctx, "GET", "http://localhost/api/admin/status", nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create request to moonlight-web")
		http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Add moonlight-web credentials
	moonlightCreds := apiServer.getMoonlightCredentials()
	httpReq.Header.Set("Authorization", "Bearer "+moonlightCreds)

	// Write HTTP request to RevDial connection
	if err := httpReq.Write(conn); err != nil {
		log.Error().Err(err).Msg("Failed to write request to RevDial connection")
		http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Read HTTP response from RevDial connection
	resp, err := http.ReadResponse(bufio.NewReader(conn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response from RevDial connection")
		http.Error(res, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(resp.StatusCode)

	// Copy response body
	if _, err := io.Copy(res, resp.Body); err != nil {
		log.Error().Err(err).Msg("Failed to copy moonlight-web status response body")
		return
	}
}

// checkStreamingRateLimit checks if a streaming connection is allowed for this session.
// Returns true if allowed, false if rate limited.
// Also updates the last connection time for the session.
func (apiServer *HelixAPIServer) checkStreamingRateLimit(sessionID string) bool {
	now := time.Now()

	apiServer.streamingRateLimiterMutex.Lock()
	defer apiServer.streamingRateLimiterMutex.Unlock()

	lastConnection, exists := apiServer.streamingRateLimiter[sessionID]
	if exists && now.Sub(lastConnection) < streamingRateLimitDuration {
		// Too soon since last connection
		log.Debug().
			Str("session_id", sessionID).
			Dur("since_last", now.Sub(lastConnection)).
			Dur("required", streamingRateLimitDuration).
			Msg("Streaming connection rate limited")
		return false
	}

	// Update last connection time
	apiServer.streamingRateLimiter[sessionID] = now

	// Clean up old entries periodically (every 100 connections, remove entries older than 1 minute)
	if len(apiServer.streamingRateLimiter) > 100 {
		for id, t := range apiServer.streamingRateLimiter {
			if now.Sub(t) > time.Minute {
				delete(apiServer.streamingRateLimiter, id)
			}
		}
	}

	return true
}
