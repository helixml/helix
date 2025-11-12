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

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClient()
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

	// Get Wolf client from the executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	wolfClient := wolfExecutor.GetWolfClient()
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
