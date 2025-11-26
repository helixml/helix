package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
)

// Wolf pairing request/response types
type CompletePairingRequest struct {
	PairSecret string `json:"pair_secret"` // Backend field name
	UUID       string `json:"uuid"`        // Frontend field name (alias for pair_secret)
	PIN        string `json:"pin"`
}

// getWolfPendingPairRequests handles GET /api/v1/wolf/pairing/pending
func (apiServer *HelixAPIServer) getWolfPendingPairRequests(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClientForSession(wolfInstanceID)
	if wolfClient == nil {
		http.Error(res, "Wolf client not available", http.StatusServiceUnavailable)
		return
	}

	pendingRequests, err := wolfClient.GetPendingPairRequests()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get pending pair requests from Wolf")
		http.Error(res, fmt.Sprintf("Failed to get pending requests: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Int("pending_count", len(pendingRequests)).
		Msg("Retrieved pending Wolf pair requests")

	// Transform to frontend-expected format
	frontendRequests := make([]map[string]interface{}, len(pendingRequests))
	for i, req := range pendingRequests {
		frontendRequests[i] = map[string]interface{}{
			"client_name": req.ClientIP,        // Use IP as name
			"uuid":        req.PairSecret,      // Use pair_secret as uuid (for completion)
			"pair_secret": req.PairSecret,      // Also include pair_secret for backend
			"client_ip":   req.ClientIP,        // Include IP
			"pin":         "",                  // Not known yet (user will enter)
			"expires_at":  0,                   // Not provided by Wolf
		}
	}

	log.Info().
		Str("user_id", user.ID).
		Interface("frontend_requests", frontendRequests).
		Msg("Sending transformed pairing requests to frontend")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(frontendRequests)
}

// completeWolfPairing handles POST /api/v1/wolf/pairing/complete
func (apiServer *HelixAPIServer) completeWolfPairing(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	var pairReq CompletePairingRequest
	err := json.NewDecoder(req.Body).Decode(&pairReq)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Validate required fields (accept either pair_secret or uuid)
	pairSecret := pairReq.PairSecret
	if pairSecret == "" {
		pairSecret = pairReq.UUID // Frontend sends uuid
	}
	if pairSecret == "" {
		http.Error(res, "pair_secret or uuid is required", http.StatusBadRequest)
		return
	}
	if pairReq.PIN == "" {
		http.Error(res, "pin is required", http.StatusBadRequest)
		return
	}

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClientForSession(wolfInstanceID)
	if wolfClient == nil {
		http.Error(res, "Wolf client not available", http.StatusServiceUnavailable)
		return
	}

	err = wolfClient.PairClient(pairSecret, pairReq.PIN)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("pair_secret", pairReq.PairSecret).
			Msg("Failed to complete Wolf pairing")
		http.Error(res, fmt.Sprintf("Pairing failed: %s", err.Error()), http.StatusBadRequest)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("pair_secret", pairReq.PairSecret).
		Msg("Wolf pairing completed successfully")

	response := map[string]interface{}{
		"success": true,
		"message": "Pairing completed successfully",
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// getWolfHealth handles GET /api/v1/wolf/health
// Returns Wolf system health including thread heartbeat status and deadlock detection
//
// @Summary Get Wolf system health
// @Description Get Wolf system health status including thread heartbeat monitoring and deadlock detection
// @Tags Wolf
// @Param wolf_instance_id query string true "Wolf instance ID to query"
// @Success 200 {object} wolf.SystemHealthResponse
// @Failure 401 {string} string "Unauthorized"
// @Failure 503 {string} string "Wolf not available"
// @Router /api/v1/wolf/health [get]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) getWolfHealth(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClientForSession(wolfInstanceID)
	if wolfClient == nil {
		http.Error(res, "Wolf client not available", http.StatusServiceUnavailable)
		return
	}

	healthResponse, err := wolfClient.GetSystemHealth(req.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get Wolf system health")
		http.Error(res, fmt.Sprintf("Failed to get system health: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("user_id", user.ID).
		Str("overall_status", healthResponse.OverallStatus).
		Int32("stuck_thread_count", healthResponse.StuckThreadCount).
		Int32("total_thread_count", healthResponse.TotalThreadCount).
		Msg("Retrieved Wolf system health")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(healthResponse)
}

// getWolfKeyboardState handles GET /api/v1/wolf/keyboard-state
// Returns keyboard state for all streaming sessions (pressed keys, modifiers)
//
// @Summary Get Wolf keyboard state
// @Description Get current keyboard state for all streaming sessions, useful for debugging stuck keys
// @Tags Wolf
// @Param wolf_instance_id query string true "Wolf instance ID to query"
// @Success 200 {object} wolf.KeyboardStateResponse
// @Failure 401 {string} string "Unauthorized"
// @Failure 503 {string} string "Wolf not available"
// @Router /api/v1/wolf/keyboard-state [get]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) getWolfKeyboardState(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClientForSession(wolfInstanceID)
	if wolfClient == nil {
		http.Error(res, "Wolf client not available", http.StatusServiceUnavailable)
		return
	}

	keyboardState, err := wolfClient.GetKeyboardState(req.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get Wolf keyboard state")
		http.Error(res, fmt.Sprintf("Failed to get keyboard state: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Debug().
		Str("user_id", user.ID).
		Int("session_count", len(keyboardState.Sessions)).
		Msg("Retrieved Wolf keyboard state")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(keyboardState)
}

// resetWolfKeyboardState handles POST /api/v1/wolf/keyboard-state/reset
// Resets keyboard state for a specific session (clears stuck keys)
//
// @Summary Reset Wolf keyboard state
// @Description Reset keyboard state for a session, releasing all stuck keys
// @Tags Wolf
// @Param wolf_instance_id query string true "Wolf instance ID to query"
// @Param session_id query string true "Session ID to reset keyboard for"
// @Success 200 {object} wolf.KeyboardResetResponse
// @Failure 401 {string} string "Unauthorized"
// @Failure 503 {string} string "Wolf not available"
// @Router /api/v1/wolf/keyboard-state/reset [post]
// @Security ApiKeyAuth
func (apiServer *HelixAPIServer) resetWolfKeyboardState(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Get Wolf instance ID from query parameter
	wolfInstanceID := req.URL.Query().Get("wolf_instance_id")
	if wolfInstanceID == "" {
		http.Error(res, "wolf_instance_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get session ID from query parameter
	sessionID := req.URL.Query().Get("session_id")
	if sessionID == "" {
		http.Error(res, "session_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClientForSession(wolfInstanceID)
	if wolfClient == nil {
		http.Error(res, "Wolf client not available", http.StatusServiceUnavailable)
		return
	}

	resetResponse, err := wolfClient.ResetKeyboardState(req.Context(), sessionID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reset Wolf keyboard state")
		http.Error(res, fmt.Sprintf("Failed to reset keyboard state: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("session_id", sessionID).
		Int("released_count", len(resetResponse.ReleasedKeys)).
		Msg("Reset Wolf keyboard state")

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(resetResponse)
}
