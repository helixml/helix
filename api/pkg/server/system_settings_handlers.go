package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// getSystemSettings godoc
// @Summary Get system settings
// @Description Get global system settings. Requires admin privileges.
// @Tags    system
// @Success 200 {object} types.SystemSettingsResponse
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/system/settings [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getSystemSettings(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	settings, err := apiServer.Store.GetSystemSettings(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("error getting system settings")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Return masked response with source information
	envToken := os.Getenv("HF_TOKEN")
	writeResponse(rw, settings.ToResponseWithSource(settings.HuggingFaceToken, envToken), http.StatusOK)
}

// updateSystemSettings godoc
// @Summary Update system settings
// @Description Update global system settings. Requires admin privileges.
// @Tags    system
// @Param request body types.SystemSettingsRequest true "System settings update"
// @Success 200 {object} types.SystemSettingsResponse
// @Failure 400 {string} string "Invalid request body"
// @Failure 403 {string} string "Forbidden - Admin required"
// @Failure 500 {string} string "Internal server error"
// @Router /api/v1/system/settings [put]
// @Security BearerAuth
func (apiServer *HelixAPIServer) updateSystemSettings(rw http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if !isAdmin(user) {
		http.Error(rw, "Forbidden: Admin privileges required", http.StatusForbidden)
		return
	}

	var req types.SystemSettingsRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		log.Error().Err(err).Msg("error decoding updateSystemSettings request body")
		http.Error(rw, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	settings, err := apiServer.Store.UpdateSystemSettings(r.Context(), &req)
	if err != nil {
		log.Error().Err(err).Msg("error updating system settings")
		http.Error(rw, "Internal server error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("admin_user", user.ID).
		Bool("hf_token_updated", req.HuggingFaceToken != nil).
		Bool("kodit_model_updated", req.KoditEnrichmentProvider != nil || req.KoditEnrichmentModel != nil).
		Msg("system settings updated by admin")

	// Push updated settings to all connected runners
	if apiServer.scheduler != nil {
		runnerController := apiServer.scheduler.GetRunnerController()
		if runnerController != nil {
			go func() {
				// Run in goroutine to avoid blocking the API response
				runnerController.SyncSystemSettingsToAllRunners(r.Context())
				log.Info().Msg("initiated system settings sync to all runners")
			}()
		}
	}

	// Return masked response with source information (same format as GET)
	envToken := os.Getenv("HF_TOKEN")
	writeResponse(rw, settings.ToResponseWithSource(settings.HuggingFaceToken, envToken), http.StatusOK)
}
