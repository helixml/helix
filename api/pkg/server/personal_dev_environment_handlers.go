package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	external_agent "github.com/helixml/helix/api/pkg/external-agent"
	"github.com/helixml/helix/api/pkg/wolf"
)

// Personal dev environment request/response types
type CreatePersonalDevEnvironmentRequest struct {
	EnvironmentName string `json:"environment_name"`
	AppID           string `json:"app_id"`
	Description     string `json:"description,omitempty"`

	// Display configuration for the streaming session
	DisplayWidth    int `json:"display_width,omitempty"`    // Default: 2360 (iPad Pro)
	DisplayHeight   int `json:"display_height,omitempty"`   // Default: 1640 (iPad Pro)
	DisplayFPS      int `json:"display_fps,omitempty"`      // Default: 120
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
	PairSecret string `json:"pair_secret"` // Backend field name
	UUID       string `json:"uuid"`        // Frontend field name (alias for pair_secret)
	PIN        string `json:"pin"`
}

// listPersonalDevEnvironments handles GET /api/v1/personal-dev-environments
// @Summary List personal development environments
// @Description Get all personal development environments for the current user
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Success 200 {array} PersonalDevEnvironmentResponse
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments [get]
func (apiServer *HelixAPIServer) listPersonalDevEnvironments(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Use executor interface directly (works with both AppWolfExecutor and WolfExecutor)
	environments, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironments(req.Context(), user.ID)
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
// @Summary Create a personal development environment
// @Description Create a new personal development environment with the specified configuration
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Param request body CreatePersonalDevEnvironmentRequest true "Personal dev environment configuration"
// @Success 201 {object} PersonalDevEnvironmentResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments [post]
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

	// Set default display parameters if not provided
	displayWidth := createReq.DisplayWidth
	if displayWidth == 0 {
		displayWidth = 2360 // iPad Pro width
	}
	displayHeight := createReq.DisplayHeight
	if displayHeight == 0 {
		displayHeight = 1640 // iPad Pro height
	}
	displayFPS := createReq.DisplayFPS
	if displayFPS == 0 {
		displayFPS = 120 // High refresh rate
	}

	log.Info().
		Str("user_id", user.ID).
		Str("environment_name", createReq.EnvironmentName).
		Str("app_id", createReq.AppID).
		Int("display_width", displayWidth).
		Int("display_height", displayHeight).
		Int("display_fps", displayFPS).
		Msg("Creating personal dev environment")

	// Create the environment with display configuration
	environment, err := apiServer.externalAgentExecutor.CreatePersonalDevEnvironmentWithDisplay(req.Context(), user.ID, createReq.AppID, createReq.EnvironmentName, displayWidth, displayHeight, displayFPS)
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

	// Use executor interface directly
	environment, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
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

	// Get existing environment to update
	environment, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
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
// @Summary Delete a personal development environment
// @Description Delete a personal development environment by ID
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Param environmentID path string true "Environment ID"
// @Success 204 "No Content"
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments/{environmentID} [delete]
func (apiServer *HelixAPIServer) deletePersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Use executor interface directly
	err := apiServer.externalAgentExecutor.StopPersonalDevEnvironment(req.Context(), user.ID, environmentID)
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
// @Summary Start a personal development environment
// @Description Start a personal development environment by ID
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Param environmentID path string true "Environment ID"
// @Success 200 {object} PersonalDevEnvironmentResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments/{environmentID}/start [post]
func (apiServer *HelixAPIServer) startPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Get environment and restart if needed
	environment, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
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
// @Summary Stop a personal development environment
// @Description Stop a personal development environment by ID
// @Tags PersonalDevEnvironments
// @Accept json
// @Produce json
// @Param environmentID path string true "Environment ID"
// @Success 200 {object} PersonalDevEnvironmentResponse
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Security ApiKeyAuth
// @Router /api/v1/personal-dev-environments/{environmentID}/stop [post]
func (apiServer *HelixAPIServer) stopPersonalDevEnvironment(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Get environment and stop
	environment, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
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

	// Get Wolf client from executor (works with both AppWolfExecutor and WolfExecutor)
	wolfClient := getWolfClientFromExecutor(apiServer.externalAgentExecutor)
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

	// Get Wolf client from executor (works with both AppWolfExecutor and WolfExecutor)
	wolfClient := getWolfClientFromExecutor(apiServer.externalAgentExecutor)
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


// getPersonalDevEnvironmentScreenshot handles GET /api/v1/personal-dev-environments/{environmentID}/screenshot
func (apiServer *HelixAPIServer) getPersonalDevEnvironmentScreenshot(res http.ResponseWriter, req *http.Request) {
	user := getRequestUser(req)
	if user == nil {
		http.Error(res, "unauthorized", http.StatusUnauthorized)
		return
	}

	vars := mux.Vars(req)
	environmentID := vars["environmentID"]

	// Get the Personal Dev Environment instance to verify ownership
	instance, err := apiServer.externalAgentExecutor.GetPersonalDevEnvironment(req.Context(), user.ID, environmentID)
	if err != nil {
		log.Error().Err(err).Str("environment_id", environmentID).Msg("Failed to get Personal Dev Environment")
		http.Error(res, "Personal dev environment not found", http.StatusNotFound)
		return
	}

	// Use the container name - DNS will resolve via Docker hostname setting
	containerName := instance.ContainerName

	log.Info().
		Str("user_id", user.ID).
		Str("environment_id", environmentID).
		Str("container_name", containerName).
		Msg("Requesting screenshot from container screenshot server")

	// Make HTTP request to screenshot server inside the container
	screenshotURL := fmt.Sprintf("http://%s:9876/screenshot", containerName)

	screenshotReq, err := http.NewRequestWithContext(req.Context(), "GET", screenshotURL, nil)
	if err != nil {
		log.Error().Err(err).Str("container_name", containerName).Msg("Failed to create screenshot request")
		http.Error(res, "Failed to create screenshot request", http.StatusInternalServerError)
		return
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	screenshotResp, err := httpClient.Do(screenshotReq)
	if err != nil {
		log.Error().Err(err).Str("container_name", containerName).Msg("Failed to get screenshot from container")
		http.Error(res, "Failed to retrieve screenshot", http.StatusInternalServerError)
		return
	}
	defer screenshotResp.Body.Close()

	// Check screenshot server response status
	if screenshotResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(screenshotResp.Body)
		log.Error().
			Int("status", screenshotResp.StatusCode).
			Str("body", string(body)).
			Str("container_name", containerName).
			Msg("Screenshot server returned error")
		http.Error(res, "Failed to retrieve screenshot from container", screenshotResp.StatusCode)
		return
	}

	// Read PNG data from screenshot server
	pngData, err := io.ReadAll(screenshotResp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read screenshot data")
		http.Error(res, "Failed to read screenshot data", http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("environment_id", environmentID).
		Str("container_name", containerName).
		Int("size_bytes", len(pngData)).
		Msg("Successfully retrieved screenshot from container")

	// Return PNG image
	res.Header().Set("Content-Type", "image/png")
	res.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	res.WriteHeader(http.StatusOK)
	res.Write(pngData)
}

// getWolfClientFromExecutor extracts Wolf client from executor interface
// Works with both AppWolfExecutor and WolfExecutor (lobby-based)
func getWolfClientFromExecutor(executor external_agent.Executor) *wolf.Client {
	// Try AppWolfExecutor first (apps mode)
	if appExecutor, ok := executor.(*external_agent.AppWolfExecutor); ok {
		return appExecutor.GetWolfClient()
	}

	// Try WolfExecutor (lobby mode)
	if wolfExecutor, ok := executor.(*external_agent.WolfExecutor); ok {
		return wolfExecutor.GetWolfClient()
	}

	return nil
}
