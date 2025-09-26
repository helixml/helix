package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
)

// Personal dev environment request/response types
type CreatePersonalDevEnvironmentRequest struct {
	EnvironmentName string `json:"environment_name"`
	AppID           string `json:"app_id"`
	Description     string `json:"description,omitempty"`
}

type UpdatePersonalDevEnvironmentRequest struct {
	EnvironmentName string `json:"environment_name,omitempty"`
	Description     string `json:"description,omitempty"`
}

type PersonalDevEnvironmentResponse struct {
	*external_agent.ZedInstanceInfo
	Description string `json:"description,omitempty"`
	StreamURL   string `json:"stream_url,omitempty"`
}

// Wolf pairing request/response types
type CompletePairingRequest struct {
	PairSecret string `json:"pair_secret"`
	PIN        string `json:"pin"`
}

// listPersonalDevEnvironments handles GET /api/v1/personal-dev-environments
func (apiServer *HelixAPIServer) listPersonalDevEnvironments(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	environments, err := wolfExecutor.GetPersonalDevEnvironments(req.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to list personal dev environments")
		http.Error(res, fmt.Sprintf("Failed to list environments: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Convert to response format
	responses := make([]PersonalDevEnvironmentResponse, len(environments))
	for i, env := range environments {
		responses[i] = PersonalDevEnvironmentResponse{
			ZedInstanceInfo: env,
			StreamURL:       env.StreamURL,
		}
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(responses)
}

// createPersonalDevEnvironment handles POST /api/v1/personal-dev-environments
func (apiServer *HelixAPIServer) createPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	var createReq CreatePersonalDevEnvironmentRequest
	err := json.NewDecoder(req.Body).Decode(&createReq)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if createReq.EnvironmentName == "" {
		http.Error(res, "environment_name is required", http.StatusBadRequest)
		return
	}
	if createReq.AppID == "" {
		http.Error(res, "app_id is required", http.StatusBadRequest)
		return
	}

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("environment_name", createReq.EnvironmentName).
		Str("app_id", createReq.AppID).
		Msg("Creating personal dev environment")

	// Create the environment
	environment, err := wolfExecutor.CreatePersonalDevEnvironment(req.Context(), user.ID, createReq.AppID, createReq.EnvironmentName)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("environment_name", createReq.EnvironmentName).
			Msg("Failed to create personal dev environment")
		http.Error(res, fmt.Sprintf("Failed to create environment: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	response := PersonalDevEnvironmentResponse{
		ZedInstanceInfo: environment,
		Description:     createReq.Description,
		StreamURL:       environment.StreamURL,
	}

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environment.InstanceID).
		Str("stream_url", environment.StreamURL).
		Msg("Personal dev environment created successfully")

	res.Header().Set("Content-Type", "application/json")
	res.WriteHeader(http.StatusCreated)
	json.NewEncoder(res).Encode(response)
}

// getPersonalDevEnvironment handles GET /api/v1/personal-dev-environments/{environmentID}
func (apiServer *HelixAPIServer) getPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	environment, err := wolfExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("environment_id", environmentID).
			Msg("Failed to get personal dev environment")
		http.Error(res, fmt.Sprintf("Failed to get environment: %s", err.Error()), http.StatusNotFound)
		return
	}

	response := PersonalDevEnvironmentResponse{
		ZedInstanceInfo: environment,
		StreamURL:       environment.StreamURL,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// updatePersonalDevEnvironment handles PUT /api/v1/personal-dev-environments/{environmentID}
func (apiServer *HelixAPIServer) updatePersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	var updateReq UpdatePersonalDevEnvironmentRequest
	err := json.NewDecoder(req.Body).Decode(&updateReq)
	if err != nil {
		http.Error(res, fmt.Sprintf("invalid JSON: %s", err.Error()), http.StatusBadRequest)
		return
	}

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	// Get existing environment to update
	environment, err := wolfExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		http.Error(res, fmt.Sprintf("Environment not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Update fields
	if updateReq.EnvironmentName != "" {
		environment.EnvironmentName = updateReq.EnvironmentName
	}

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environmentID).
		Msg("Updated personal dev environment")

	response := PersonalDevEnvironmentResponse{
		ZedInstanceInfo: environment,
		Description:     updateReq.Description,
		StreamURL:       environment.StreamURL,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// deletePersonalDevEnvironment handles DELETE /api/v1/personal-dev-environments/{environmentID}
func (apiServer *HelixAPIServer) deletePersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	err := wolfExecutor.StopPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		log.Error().Err(err).
			Str("user_id", user.ID).
			Str("environment_id", environmentID).
			Msg("Failed to delete personal dev environment")
		http.Error(res, fmt.Sprintf("Failed to delete environment: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environmentID).
		Msg("Personal dev environment deleted successfully")

	res.WriteHeader(http.StatusNoContent)
}

// startPersonalDevEnvironment handles POST /api/v1/personal-dev-environments/{environmentID}/start
func (apiServer *HelixAPIServer) startPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	// Get environment and restart if needed
	environment, err := wolfExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		http.Error(res, fmt.Sprintf("Environment not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Update status to running (in real implementation, this would start the Wolf app)
	environment.Status = "running"
	environment.LastActivity = time.Now()

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environmentID).
		Msg("Personal dev environment started")

	response := PersonalDevEnvironmentResponse{
		ZedInstanceInfo: environment,
		StreamURL:       environment.StreamURL,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
}

// stopPersonalDevEnvironment handles POST /api/v1/personal-dev-environments/{environmentID}/stop
func (apiServer *HelixAPIServer) stopPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Check if we have Wolf executor
	wolfExecutor, ok := apiServer.externalAgentExecutor.(*external_agent.WolfExecutor)
	if !ok {
		http.Error(res, "Wolf executor not available", http.StatusServiceUnavailable)
		return
	}

	// Get environment and stop
	environment, err := wolfExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		http.Error(res, fmt.Sprintf("Environment not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Update status to stopped (in real implementation, this would stop the Wolf app)
	environment.Status = "stopped"

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environmentID).
		Msg("Personal dev environment stopped")

	response := PersonalDevEnvironmentResponse{
		ZedInstanceInfo: environment,
		StreamURL:       environment.StreamURL,
	}

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(response)
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

	res.Header().Set("Content-Type", "application/json")
	json.NewEncoder(res).Encode(pendingRequests)
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

	// Validate required fields
	if pairReq.PairSecret == "" {
		http.Error(res, "pair_secret is required", http.StatusBadRequest)
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

	err = wolfClient.PairClient(pairReq.PairSecret, pairReq.PIN)
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
