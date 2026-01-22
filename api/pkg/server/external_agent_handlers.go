package server

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/proxy"
	"github.com/helixml/helix/api/pkg/types"
)

// addUserAPITokenToAgent adds a session-scoped ephemeral API token to agent environment.
// The token is minted when the desktop starts and revoked when it shuts down.
// This ensures RBAC is enforced - agent can only access repos the user can access.
// Uses getAPIKeyForSession for consistent token selection logic across the codebase.
func (apiServer *HelixAPIServer) addUserAPITokenToAgent(ctx context.Context, agent *types.DesktopAgent, userID string) error {
	if agent.SessionID == "" {
		return fmt.Errorf("agent has no SessionID - session must be created before adding API token")
	}

	// Get the session to use for session-scoped API key
	session, err := apiServer.Store.GetSession(ctx, agent.SessionID)
	if err != nil {
		return fmt.Errorf("failed to get session %s: %w", agent.SessionID, err)
	}

	// Get session-scoped ephemeral API key
	apiKey, err := apiServer.getAPIKeyForSession(ctx, session)
	if err != nil {
		return fmt.Errorf("failed to get session API key for external agent: %w", err)
	}

	// Add API tokens to agent environment
	// These are appended LAST in hydra_executor.go, overriding runner token defaults
	agent.Env = append(agent.Env, types.DesktopAgentAPIEnvVars(apiKey)...)

	log.Debug().
		Str("user_id", userID).
		Str("session_id", agent.SessionID).
		Str("spec_task_id", agent.SpecTaskID).
		Msg("Added session-scoped API tokens to agent for git and LLM operations")

	return nil
}

// RegisterRequestToSessionMapping registers a request_id to session_id mapping for external agent sessions
// This is used to route initial messages to Zed when it connects via WebSocket
func (apiServer *HelixAPIServer) RegisterRequestToSessionMapping(requestID, sessionID string) {
	apiServer.contextMappingsMutex.Lock()
	if apiServer.requestToSessionMapping == nil {
		apiServer.requestToSessionMapping = make(map[string]string)
	}
	apiServer.requestToSessionMapping[requestID] = sessionID
	apiServer.contextMappingsMutex.Unlock()
	log.Info().
		Str("request_id", requestID).
		Str("session_id", sessionID).
		Msg("✅ Registered request_id -> session_id mapping")
}

// isExternalAgentConnected checks if external agent is connected via WebSocket
func (apiServer *HelixAPIServer) isExternalAgentConnected(sessionID string) bool {
	_, exists := apiServer.externalAgentWSManager.getConnection(sessionID)
	return exists
}

// sendCommandToExternalAgentHandler allows manual command sending for testing
func (apiServer *HelixAPIServer) sendCommandToExternalAgentHandler(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	if sessionID == "" {
		http.Error(res, "session ID is required", http.StatusBadRequest)
		return
	}

	var command types.ExternalAgentCommand
	err := json.NewDecoder(req.Body).Decode(&command)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if err := apiServer.sendCommandToExternalAgent(sessionID, command); err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("failed to send command to external agent")
		http.Error(res, fmt.Sprintf("failed to send command: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := map[string]string{
		"message":    "Command sent successfully",
		"session_id": sessionID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getExternalAgentScreenshot handles GET /api/v1/external-agents/{sessionID}/screenshot
func (apiServer *HelixAPIServer) getExternalAgentScreenshot(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if agent is paused and has saved screenshot
	if session.Metadata.PausedScreenshotPath != "" {
		// Agent is paused - serve saved screenshot from filestore
		screenshotFile, err := os.Open(session.Metadata.PausedScreenshotPath)
		if err == nil {
			defer screenshotFile.Close()
			res.Header().Set("Content-Type", "image/png")
			res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			res.Header().Set("X-Paused-Screenshot", "true") // Indicate this is a paused screenshot
			res.WriteHeader(http.StatusOK)
			io.Copy(res, screenshotFile)
			return
		}
		// If file not found, fall through to try live screenshot
		log.Warn().Err(err).Str("screenshot_path", session.Metadata.PausedScreenshotPath).Msg("Paused screenshot file not found, trying live screenshot")
	}

	// Try RevDial connection to desktop container (registered as "desktop-{session_id}")
	// RevDial is the primary communication mechanism for desktop container access
	// Note: "desktop-" prefix is for per-session containers, "sandbox-" is for the outer sandbox
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP request over RevDial tunnel
	// Forward query parameters (format, quality, include_cursor) to the desktop container
	screenshotURL := "http://localhost:9876/screenshot"
	if req.URL.RawQuery != "" {
		screenshotURL += "?" + req.URL.RawQuery
	}
	httpReq, err := http.NewRequest("GET", screenshotURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create screenshot request")
		http.Error(res, "Failed to create screenshot request", http.StatusInternalServerError)
		return
	}

	// Write request to RevDial connection
	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write request to RevDial connection")
		http.Error(res, "Failed to send screenshot request", http.StatusInternalServerError)
		return
	}

	// Read response from RevDial connection
	screenshotResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read screenshot response from RevDial")
		http.Error(res, "Failed to read screenshot response", http.StatusInternalServerError)
		return
	}
	defer screenshotResp.Body.Close()

	// Check screenshot server response status
	if screenshotResp.StatusCode != http.StatusOK {
		// Read response body for debugging
		errorBody, _ := io.ReadAll(screenshotResp.Body)
		log.Error().
			Int("status", screenshotResp.StatusCode).
			Str("session_id", sessionID).
			Str("error_body", string(errorBody)).
			Msg("Screenshot server returned error")
		http.Error(res, "Failed to retrieve screenshot from container", screenshotResp.StatusCode)
		return
	}

	// Return PNG image directly
	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.WriteHeader(http.StatusOK)

	// Stream the PNG data from screenshot server to response
	_, err = io.Copy(res, screenshotResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream screenshot data")
		return
	}

}

// @Summary Execute command in sandbox
// @Description Executes a command inside the sandbox container for benchmarking and debugging.
// @Description Only specific safe commands are allowed (vkcube, glxgears, pkill).
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param body body object true "Command to execute" Example({"command": ["vkcube"], "background": true})
// @Success 200 {object} object "Execution result"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/exec [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) execInSandbox(res http.ResponseWriter, req *http.Request) {
	log.Info().Str("path", req.URL.Path).Str("method", req.Method).Msg("🔧 execInSandbox handler called")
	user := getRequestUser(req)
	if user == nil {
		log.Info().Msg("🔧 execInSandbox: no user found")
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}
	log.Info().Str("user_id", user.ID).Msg("🔧 execInSandbox: user authenticated")

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]
	log.Info().Str("session_id", sessionID).Msg("🔧 execInSandbox: extracted sessionID")

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("🔧 execInSandbox: Failed to get session")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}
	log.Info().Str("session_id", session.ID).Str("owner", session.Owner).Msg("🔧 execInSandbox: session found")

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("🔧 execInSandbox: User does not own session")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}
	log.Info().Msg("🔧 execInSandbox: ownership verified")

	// Read request body
	log.Info().Msg("🔧 execInSandbox: reading request body")
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("🔧 execInSandbox: failed to read body")
		http.Error(res, "Failed to read request body", http.StatusBadRequest)
		return
	}
	log.Info().Int("body_len", len(bodyBytes)).Msg("🔧 execInSandbox: body read successfully")

	// Connect to desktop container via RevDial
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	log.Info().Str("runner_id", runnerID).Msg("🔧 execInSandbox: connecting via RevDial")
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("🔧 execInSandbox: Failed to connect to desktop container via RevDial for exec")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()
	log.Info().Msg("🔧 execInSandbox: RevDial connected")

	// Send POST request to /exec over RevDial tunnel
	log.Info().Msg("🔧 execInSandbox: creating HTTP request")
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/exec", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Error().Err(err).Msg("🔧 execInSandbox: Failed to create exec request")
		http.Error(res, "Failed to create exec request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Write request to RevDial connection
	log.Info().Msg("🔧 execInSandbox: writing request to RevDial")
	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("🔧 execInSandbox: Failed to write exec request to RevDial connection")
		http.Error(res, "Failed to send exec request", http.StatusInternalServerError)
		return
	}
	log.Info().Msg("🔧 execInSandbox: request written, reading response")

	// Read response from RevDial connection
	execResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("🔧 execInSandbox: Failed to read exec response from RevDial")
		http.Error(res, "Failed to read exec response", http.StatusInternalServerError)
		return
	}
	defer execResp.Body.Close()
	log.Info().Int("status_code", execResp.StatusCode).Msg("🔧 execInSandbox: got response from sandbox")

	// Forward response to client
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(execResp.StatusCode)
	log.Info().Int("status_code", execResp.StatusCode).Msg("🔧 execInSandbox: forwarding response to client")
	io.Copy(res, execResp.Body)
}

// @Summary Bandwidth probe for adaptive bitrate
// @Description Returns random uncompressible data for measuring available bandwidth.
// @Description Used by adaptive bitrate algorithm to probe network throughput.
// @Description Only requires authentication, not session ownership.
// @Tags ExternalAgents
// @Produce application/octet-stream
// @Param size query int false "Size of data to return in bytes (default 524288 = 512KB, max 2MB)"
// @Success 200 {file} binary
// @Failure 401 {object} system.HTTPError
// @Router /api/v1/bandwidth-probe [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getBandwidthProbe(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse size parameter (default 512KB, max 2MB to limit abuse)
	size := 524288 // 512KB default
	if sizeStr := req.URL.Query().Get("size"); sizeStr != "" {
		if parsedSize, err := strconv.Atoi(sizeStr); err == nil && parsedSize > 0 && parsedSize <= 2*1024*1024 {
			size = parsedSize // Max 2MB
		}
	}

	// Generate random data - crypto/rand produces incompressible data
	// This ensures we're measuring actual bandwidth, not compression efficiency
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		log.Error().Err(err).Msg("Failed to generate random data for bandwidth probe")
		http.Error(res, "Failed to generate probe data", http.StatusInternalServerError)
		return
	}

	// Set headers to prevent caching and compression
	res.Header().Set("Content-Type", "application/octet-stream")
	res.Header().Set("Content-Length", strconv.Itoa(size))
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.Header().Set("Content-Encoding", "identity") // Explicitly disable compression
	res.WriteHeader(http.StatusOK)

	res.Write(data)
}

// @Summary Get session clipboard content
// @Description Fetch current clipboard content from remote desktop
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Success 200 {object} types.ClipboardData
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/clipboard [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getExternalAgentClipboard(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Executor not available", http.StatusServiceUnavailable)
		return
	}

	containerName, err := apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP request over RevDial tunnel
	httpReq, err := http.NewRequest("GET", "http://localhost:9876/clipboard", nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create clipboard request")
		http.Error(res, "Failed to create clipboard request", http.StatusInternalServerError)
		return
	}

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write clipboard request to RevDial")
		http.Error(res, "Failed to send clipboard request", http.StatusInternalServerError)
		return
	}

	clipboardResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read clipboard response from RevDial")
		http.Error(res, "Failed to read clipboard response", http.StatusInternalServerError)
		return
	}
	defer clipboardResp.Body.Close()

	// Check clipboard server response status
	if clipboardResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", clipboardResp.StatusCode).
			Str("container_name", containerName).
			Msg("Clipboard server returned error")
		http.Error(res, "Failed to retrieve clipboard from container", clipboardResp.StatusCode)
		return
	}

	// Return clipboard data directly (JSON format with type and data)
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)

	// Stream the clipboard JSON from clipboard server to response
	_, err = io.Copy(res, clipboardResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream clipboard data")
		return
	}

	log.Trace().
		Str("session_id", sessionID).
		Msg("Successfully retrieved clipboard from external agent container")
}

// @Summary Set session clipboard content
// @Description Send clipboard content to remote desktop
// @Tags ExternalAgents
// @Accept json
// @Param sessionID path string true "Session ID"
// @Param clipboard body types.ClipboardData true "Clipboard data to set"
// @Success 200
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/clipboard [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) setExternalAgentClipboard(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Executor not available", http.StatusServiceUnavailable)
		return
	}

	containerName, err := apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Read clipboard content from request body (JSON format)
	clipboardContent, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read request body")
		http.Error(res, "Failed to read clipboard content", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("container_name", containerName).
		Int("clipboard_size", len(clipboardContent)).
		Msg("Setting clipboard in sandbox via RevDial")

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/clipboard", bytes.NewReader(clipboardContent))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create clipboard POST request")
		http.Error(res, "Failed to create clipboard request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write clipboard POST to RevDial")
		http.Error(res, "Failed to send clipboard request", http.StatusInternalServerError)
		return
	}

	clipboardResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read clipboard POST response from RevDial")
		http.Error(res, "Failed to set clipboard", http.StatusInternalServerError)
		return
	}
	defer clipboardResp.Body.Close()

	// Check clipboard server response status
	if clipboardResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", clipboardResp.StatusCode).
			Str("container_name", containerName).
			Msg("Clipboard server returned error")
		http.Error(res, "Failed to set clipboard in container", clipboardResp.StatusCode)
		return
	}

	res.WriteHeader(http.StatusOK)
	log.Info().
		Str("session_id", sessionID).
		Int("clipboard_size", len(clipboardContent)).
		Msg("Successfully set clipboard in external agent container")
}

// @Summary Send input events to sandbox
// @Description Send keyboard and mouse input events to the remote desktop. Supports single events or batches.
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param input body object true "Input event(s). Single event: {type, keycode, state} or batch: {events: [...]}"
// @Success 200 {object} object "success response with processed count"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/input [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) sendInputToSandbox(res http.ResponseWriter, req *http.Request) {
	log.Info().Str("path", req.URL.Path).Str("method", req.Method).Msg("🔧 sendInputToSandbox handler called")
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for input")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for input")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Executor not available", http.StatusServiceUnavailable)
		return
	}

	_, err = apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container for input")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Read input content from request body
	inputContent, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input request body")
		http.Error(res, "Failed to read input content", http.StatusBadRequest)
		return
	}

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for input")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/input", bytes.NewReader(inputContent))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create input request")
		http.Error(res, "Failed to create input request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write input request to RevDial")
		http.Error(res, "Failed to send input request", http.StatusInternalServerError)
		return
	}

	inputResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input response from RevDial")
		http.Error(res, "Failed to read input response", http.StatusInternalServerError)
		return
	}
	defer inputResp.Body.Close()

	// Read and forward response
	respBody, err := io.ReadAll(inputResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read input response body")
		http.Error(res, "Failed to read input response", http.StatusInternalServerError)
		return
	}

	// Forward status and body
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(inputResp.StatusCode)
	res.Write(respBody)

	log.Trace().
		Str("session_id", sessionID).
		Int("input_size", len(inputContent)).
		Int("status", inputResp.StatusCode).
		Msg("Input event(s) sent to sandbox")
}

// @Summary Upload file to sandbox
// @Description Upload a file to the sandbox incoming folder (~/work/incoming/). Files can be dragged and dropped onto the sandbox viewer to upload them.
// @Tags ExternalAgents
// @Accept multipart/form-data
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param file formData file true "File to upload"
// @Param open_file_manager query bool false "Open file manager to show uploaded file (default: true)"
// @Success 200 {object} types.SandboxFileUploadResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/upload [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) uploadFileToSandbox(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for file upload")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for file upload")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get container name using executor
	if apiServer.externalAgentExecutor == nil {
		http.Error(res, "Executor not available", http.StatusServiceUnavailable)
		return
	}

	_, err = apiServer.externalAgentExecutor.FindContainerBySessionID(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to find external agent container for file upload")
		http.Error(res, "External agent container not found", http.StatusNotFound)
		return
	}

	// Read the multipart body to forward it
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload request body")
		http.Error(res, "Failed to read file", http.StatusBadRequest)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Int("body_size", len(bodyBytes)).
		Str("content_type", req.Header.Get("Content-Type")).
		Msg("Uploading file to sandbox via RevDial")

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for file upload")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel
	// Important: preserve the Content-Type header with multipart boundary
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/upload", bytes.NewReader(bodyBytes))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create upload request")
		http.Error(res, "Failed to create upload request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", req.Header.Get("Content-Type"))
	httpReq.ContentLength = int64(len(bodyBytes))

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write upload request to RevDial")
		http.Error(res, "Failed to send upload request", http.StatusInternalServerError)
		return
	}

	uploadResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload response from RevDial")
		http.Error(res, "Failed to read upload response", http.StatusInternalServerError)
		return
	}
	defer uploadResp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read upload response body")
		http.Error(res, "Failed to read upload response", http.StatusInternalServerError)
		return
	}

	// Check upload server response status
	if uploadResp.StatusCode != http.StatusOK {
		log.Error().
			Int("status", uploadResp.StatusCode).
			Str("response", string(respBody)).
			Msg("Screenshot server returned error for upload")
		http.Error(res, string(respBody), uploadResp.StatusCode)
		return
	}

	// Return the response from screenshot server
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)
	res.Write(respBody)

	log.Info().
		Str("session_id", sessionID).
		Str("response", string(respBody)).
		Msg("Successfully uploaded file to sandbox")
}

// ConfigurePendingSessionRequest is the request body for configuring a pending session
type ConfigurePendingSessionRequest struct {
	ClientUniqueID string `json:"client_unique_id"`
}

// configurePendingSession handles POST /api/v1/external-agents/{sessionID}/configure-pending-session
// @Summary Configure pending session (deprecated - no-op)
// @Description This endpoint was used for session configuration but is now a no-op.
// Kept for API compatibility.
// @Tags ExternalAgents
// @Accept json
// @Produce json
// @Param sessionID path string true "External agent session ID"
// @Param request body ConfigurePendingSessionRequest true "Configuration request"
// @Success 200 {object} map[string]string "success response"
// @Failure 401 {string} string "Unauthorized"
// @Router /api/v1/external-agents/{sessionID}/configure-pending-session [post]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) configurePendingSession(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Parse request body (for compatibility)
	var configReq ConfigurePendingSessionRequest
	if err := json.NewDecoder(req.Body).Decode(&configReq); err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// No-op: Session configuration is no longer needed
	// Just return success for API compatibility
	response := map[string]string{
		"status":           "configured",
		"session_id":       sessionID,
		"client_unique_id": configReq.ClientUniqueID,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// proxyInputWebSocket handles WebSocket /api/v1/external-agents/{sessionID}/ws/input
// This provides direct input from browser to screenshot-server.
// @Summary Direct WebSocket input for PipeWire/GNOME sessions
// @Description Provides a WebSocket connection for sending input events directly to the screenshot-server
// in the sandbox. This provides direct control over input events.
// Only available for PipeWire/GNOME desktop sessions.
// @Tags ExternalAgents
// @Param sessionID path string true "Session ID"
// @Success 101 "Switching Protocols"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/ws/input [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) proxyInputWebSocket(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for input WebSocket")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for input WebSocket")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Check if this is a PipeWire/GNOME session (Ubuntu desktop)
	// For Sway sessions, return an error - Sway has different input handling
	var desktopType string
	if session.Metadata.ExternalAgentConfig != nil {
		desktopType = session.Metadata.ExternalAgentConfig.GetEffectiveDesktopType()
	} else {
		desktopType = "ubuntu" // Default to ubuntu if no config
	}
	if desktopType != "ubuntu" {
		log.Warn().Str("session_id", sessionID).Str("desktop_type", desktopType).Msg("Direct input WebSocket not supported for non-Ubuntu sessions")
		http.Error(res, "Direct input WebSocket only supported for Ubuntu/GNOME sessions", http.StatusNotImplemented)
		return
	}

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Msg("Proxying input WebSocket to screenshot-server via RevDial")

	// Hijack the HTTP connection to get the underlying net.Conn
	hijacker, ok := res.(http.Hijacker)
	if !ok {
		log.Error().Msg("ResponseWriter doesn't support Hijacker interface")
		http.Error(res, "Server doesn't support connection hijacking", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error().Err(err).Msg("Failed to hijack connection")
		http.Error(res, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Get RevDial connection to the screenshot-server
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	serverConn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for input WebSocket")
		// Write HTTP error response since we've already hijacked
		clientConn.Write([]byte("HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\n\r\nSandbox not connected"))
		return
	}
	defer serverConn.Close()

	// Construct WebSocket upgrade request to forward to screenshot-server
	upgradeReq := fmt.Sprintf("GET /ws/input HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", req.Header.Get("Sec-WebSocket-Key"))

	// Forward the WebSocket upgrade request
	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade to screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nFailed to connect to screenshot-server"))
		return
	}

	// Read the upgrade response from screenshot-server
	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read WebSocket upgrade response from screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nScreenshot-server connection failed"))
		return
	}
	defer upgradeResp.Body.Close()

	// Check if upgrade was successful
	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Error().Int("status", upgradeResp.StatusCode).Msg("Screenshot-server didn't accept WebSocket upgrade")
		clientConn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", upgradeResp.StatusCode, upgradeResp.Status)))
		return
	}

	// Forward the 101 Switching Protocols response to the client
	upgradeRespBytes := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n"+
		"\r\n", upgradeResp.Header.Get("Sec-WebSocket-Accept"))

	if _, err := clientConn.Write([]byte(upgradeRespBytes)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade response to client")
		return
	}

	log.Info().Str("session_id", sessionID).Msg("Input WebSocket connection established, starting resilient proxy")

	// Generate a unique proxy session ID
	proxySessionID := generateProxySessionID()

	// Create dial function that uses connman with grace period support
	dialFunc := func(ctx context.Context) (net.Conn, error) {
		return apiServer.connman.Dial(ctx, runnerID)
	}

	// Create upgrade function for WebSocket
	wsKey := req.Header.Get("Sec-WebSocket-Key")
	upgradeFunc := proxy.CreateWebSocketUpgradeFunc("/ws/input", wsKey)

	// Create resilient proxy
	resilientProxy := proxy.NewResilientProxy(proxy.ResilientProxyConfig{
		SessionID:   proxySessionID,
		ClientConn:  clientConn,
		ServerConn:  serverConn,
		DialFunc:    dialFunc,
		UpgradeFunc: upgradeFunc,
	})
	defer resilientProxy.Close()

	// Run the proxy (blocks until connection closes or error)
	if err := resilientProxy.Run(req.Context()); err != nil {
		log.Warn().
			Str("session_id", sessionID).
			Str("proxy_session_id", proxySessionID).
			Err(err).
			Msg("Resilient proxy ended with error")
	}

	stats := resilientProxy.Stats()
	log.Info().
		Str("session_id", sessionID).
		Str("proxy_session_id", proxySessionID).
		Int64("reconnect_count", stats.ReconnectCount).
		Int64("input_bytes_buffered", stats.InputBytesBuffered).
		Int64("output_bytes_buffered", stats.OutputBytesBuffered).
		Msg("Input WebSocket connection closed")
}

// generateProxySessionID generates a unique session ID for proxy connections
func generateProxySessionID() string {
	buf := make([]byte, 8)
	rand.Read(buf)
	return hex.EncodeToString(buf)
}

// proxyStreamWebSocket handles WebSocket /api/v1/external-agents/{sessionID}/ws/stream
// This provides direct video streaming from screenshot-server to browser.
// @Summary Direct WebSocket video streaming for PipeWire/GNOME sessions
// @Description Provides a WebSocket connection for receiving H.264 video frames directly from the
// screenshot-server in the sandbox.
// Only available for PipeWire/GNOME desktop sessions.
// @Tags ExternalAgents
// @Param sessionID path string true "Session ID"
// @Success 101 "Switching Protocols"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/ws/stream [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) proxyStreamWebSocket(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for stream WebSocket")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership (or streaming access grant)
	if session.Owner != user.ID && !isAdmin(user) {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not have access to session for stream WebSocket")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Get RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)

	log.Info().
		Str("session_id", sessionID).
		Str("runner_id", runnerID).
		Msg("Proxying stream WebSocket to screenshot-server via RevDial")

	// Hijack the HTTP connection to get the underlying net.Conn
	hijacker, ok := res.(http.Hijacker)
	if !ok {
		log.Error().Msg("ResponseWriter doesn't support Hijacker interface")
		http.Error(res, "Server doesn't support connection hijacking", http.StatusInternalServerError)
		return
	}

	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		log.Error().Err(err).Msg("Failed to hijack connection")
		http.Error(res, "Failed to hijack connection", http.StatusInternalServerError)
		return
	}
	defer clientConn.Close()

	// Get RevDial connection to the screenshot-server
	ctx, cancel := context.WithTimeout(req.Context(), 30*time.Second)
	defer cancel()

	serverConn, err := apiServer.connman.Dial(ctx, runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for stream WebSocket")
		// Write HTTP error response since we've already hijacked
		clientConn.Write([]byte("HTTP/1.1 503 Service Unavailable\r\nContent-Type: text/plain\r\n\r\nSandbox not connected"))
		return
	}
	defer serverConn.Close()

	// Construct WebSocket upgrade request to forward to screenshot-server
	upgradeReq := fmt.Sprintf("GET /ws/stream HTTP/1.1\r\n"+
		"Host: localhost:9876\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: %s\r\n"+
		"Sec-WebSocket-Version: 13\r\n"+
		"\r\n", req.Header.Get("Sec-WebSocket-Key"))

	// Forward the WebSocket upgrade request
	if _, err := serverConn.Write([]byte(upgradeReq)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade to screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nFailed to connect to screenshot-server"))
		return
	}

	// Read the upgrade response from screenshot-server
	serverReader := bufio.NewReader(serverConn)
	upgradeResp, err := http.ReadResponse(serverReader, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read WebSocket upgrade response from screenshot-server")
		clientConn.Write([]byte("HTTP/1.1 502 Bad Gateway\r\nContent-Type: text/plain\r\n\r\nScreenshot-server connection failed"))
		return
	}
	defer upgradeResp.Body.Close()

	// Check if upgrade was successful
	if upgradeResp.StatusCode != http.StatusSwitchingProtocols {
		log.Error().Int("status", upgradeResp.StatusCode).Msg("Screenshot-server didn't accept WebSocket upgrade")
		clientConn.Write([]byte(fmt.Sprintf("HTTP/1.1 %d %s\r\n\r\n", upgradeResp.StatusCode, upgradeResp.Status)))
		return
	}

	// Forward the 101 Switching Protocols response to the client
	upgradeRespBytes := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n"+
		"\r\n", upgradeResp.Header.Get("Sec-WebSocket-Accept"))

	if _, err := clientConn.Write([]byte(upgradeRespBytes)); err != nil {
		log.Error().Err(err).Msg("Failed to send WebSocket upgrade response to client")
		return
	}

	log.Info().Str("session_id", sessionID).Msg("Stream WebSocket connection established, starting resilient proxy")

	// Generate a unique proxy session ID
	proxySessionID := generateProxySessionID()

	// Create dial function that uses connman with grace period support
	dialFunc := func(ctx context.Context) (net.Conn, error) {
		return apiServer.connman.Dial(ctx, runnerID)
	}

	// Create upgrade function for WebSocket
	wsKey := req.Header.Get("Sec-WebSocket-Key")
	upgradeFunc := proxy.CreateWebSocketUpgradeFunc("/ws/stream", wsKey)

	// Create resilient proxy
	resilientProxy := proxy.NewResilientProxy(proxy.ResilientProxyConfig{
		SessionID:   proxySessionID,
		ClientConn:  clientConn,
		ServerConn:  serverConn,
		DialFunc:    dialFunc,
		UpgradeFunc: upgradeFunc,
	})
	defer resilientProxy.Close()

	// Run the proxy (blocks until connection closes or error)
	if err := resilientProxy.Run(req.Context()); err != nil {
		log.Warn().
			Str("session_id", sessionID).
			Str("proxy_session_id", proxySessionID).
			Err(err).
			Msg("Resilient proxy ended with error")
	}

	stats := resilientProxy.Stats()
	log.Info().
		Str("session_id", sessionID).
		Str("proxy_session_id", proxySessionID).
		Int64("reconnect_count", stats.ReconnectCount).
		Int64("input_bytes_buffered", stats.InputBytesBuffered).
		Int64("output_bytes_buffered", stats.OutputBytesBuffered).
		Msg("Stream WebSocket connection closed")
}

// @Summary Voice input to desktop
// @Description Send audio for speech-to-text transcription and type the result at cursor position
// @Tags ExternalAgents
// @Accept multipart/form-data
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param audio formData file true "Audio file (WebM/Opus format)"
// @Success 200 {object} object "Transcription result with 'text' and 'status' fields"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/voice [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) sendVoiceInput(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for voice input")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for voice input")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Parse multipart form (10MB max)
	if err := req.ParseMultipartForm(10 << 20); err != nil {
		log.Error().Err(err).Msg("Failed to parse multipart form for voice input")
		http.Error(res, "Failed to parse form", http.StatusBadRequest)
		return
	}

	// Get audio file from form
	file, _, err := req.FormFile("audio")
	if err != nil {
		log.Error().Err(err).Msg("No audio file in voice input request")
		http.Error(res, "No audio file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Read audio data
	audioData, err := io.ReadAll(file)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read audio data")
		http.Error(res, "Failed to read audio", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Int("audio_size", len(audioData)).
		Msg("Received voice input, forwarding to desktop")

	// Get RevDial connection to desktop container
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for voice input")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Send HTTP POST request over RevDial tunnel with audio data
	httpReq, err := http.NewRequest("POST", "http://localhost:9876/voice", bytes.NewReader(audioData))
	if err != nil {
		log.Error().Err(err).Msg("Failed to create voice request")
		http.Error(res, "Failed to create voice request", http.StatusInternalServerError)
		return
	}
	httpReq.Header.Set("Content-Type", "audio/webm")
	httpReq.ContentLength = int64(len(audioData))

	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write voice request to RevDial")
		http.Error(res, "Failed to send voice request", http.StatusInternalServerError)
		return
	}

	voiceResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read voice response from RevDial")
		http.Error(res, "Failed to read voice response", http.StatusInternalServerError)
		return
	}
	defer voiceResp.Body.Close()

	// Check voice server response status
	if voiceResp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(voiceResp.Body)
		log.Error().
			Int("status", voiceResp.StatusCode).
			Str("body", string(bodyBytes)).
			Msg("Voice transcription server returned error")
		http.Error(res, "Voice transcription failed", voiceResp.StatusCode)
		return
	}

	// Return transcription result
	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusOK)

	// Stream the response from voice server to client
	_, err = io.Copy(res, voiceResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream voice response")
		return
	}

	log.Info().Str("session_id", sessionID).Msg("Voice transcription completed successfully")
}

// @Summary Get file diff from container
// @Description Returns git diff information from the running desktop container.
// @Description Shows changes between the current working directory and base branch,
// @Description including uncommitted changes.
// @Tags ExternalAgents
// @Produce json
// @Param sessionID path string true "Session ID"
// @Param base query string false "Base branch to compare against (default: main)"
// @Param include_content query bool false "Include full diff content for each file (default: false)"
// @Param path query string false "Filter to specific file path"
// @Success 200 {object} object "Diff response with files list"
// @Failure 401 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 503 {object} system.HTTPError
// @Router /api/v1/external-agents/{sessionID}/diff [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getExternalAgentDiff(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	sessionID := vars["sessionID"]

	// Get the Helix session to verify ownership
	session, err := apiServer.Store.GetSession(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to get session for diff")
		http.Error(res, "Session not found", http.StatusNotFound)
		return
	}

	// Verify ownership
	if session.Owner != user.ID {
		log.Warn().Str("session_id", sessionID).Str("user_id", user.ID).Str("owner_id", session.Owner).Msg("User does not own session for diff")
		http.Error(res, "Forbidden", http.StatusForbidden)
		return
	}

	// Try RevDial connection to desktop container (registered as "desktop-{session_id}")
	runnerID := fmt.Sprintf("desktop-%s", sessionID)
	revDialConn, err := apiServer.connman.Dial(req.Context(), runnerID)
	if err != nil {
		log.Error().
			Err(err).
			Str("runner_id", runnerID).
			Str("session_id", sessionID).
			Msg("Failed to connect to sandbox via RevDial for diff")
		http.Error(res, fmt.Sprintf("Sandbox not connected: %v", err), http.StatusServiceUnavailable)
		return
	}
	defer revDialConn.Close()

	// Build the diff URL with query parameters
	diffURL := "http://localhost:9876/diff"
	if req.URL.RawQuery != "" {
		diffURL += "?" + req.URL.RawQuery
	}

	// Send HTTP request over RevDial tunnel
	httpReq, err := http.NewRequest("GET", diffURL, nil)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create diff request")
		http.Error(res, "Failed to create diff request", http.StatusInternalServerError)
		return
	}

	// Write request to RevDial connection
	if err := httpReq.Write(revDialConn); err != nil {
		log.Error().Err(err).Msg("Failed to write request to RevDial connection")
		http.Error(res, "Failed to send diff request", http.StatusInternalServerError)
		return
	}

	// Read response from RevDial connection
	diffResp, err := http.ReadResponse(bufio.NewReader(revDialConn), httpReq)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read diff response from RevDial")
		http.Error(res, "Failed to read diff response", http.StatusInternalServerError)
		return
	}
	defer diffResp.Body.Close()

	// Check response status
	if diffResp.StatusCode != http.StatusOK {
		errorBody, _ := io.ReadAll(diffResp.Body)
		log.Error().
			Int("status", diffResp.StatusCode).
			Str("session_id", sessionID).
			Str("error_body", string(errorBody)).
			Msg("Diff server returned error")
		http.Error(res, "Failed to get diff from container", diffResp.StatusCode)
		return
	}

	// Return JSON directly
	res.Header().Set("Content-Type", "application/json")
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.WriteHeader(http.StatusOK)

	// Stream the diff JSON from container to response
	_, err = io.Copy(res, diffResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to stream diff data")
		return
	}

	log.Debug().Str("session_id", sessionID).Msg("Successfully retrieved diff from container")
}
