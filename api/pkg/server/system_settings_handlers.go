package server

import (
	"context"
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
		Bool("rag_embeddings_model_updated", req.RAGEmbeddingsProvider != nil || req.RAGEmbeddingsModel != nil).
		Bool("max_concurrent_desktops_updated", req.MaxConcurrentDesktops != nil).
		Bool("providers_management_enabled_updated", req.ProvidersManagementEnabled != nil).
		Bool("enforce_quotas_updated", req.EnforceQuotas != nil).
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

	// If the admin changed a kodit embedding setting, reinitialise the kodit
	// client in-process so the new provider takes effect without a restart.
	// kodit itself handles the vector-table rebuild and repository re-index
	// when the embedding dimension changes.
	koditEmbeddingChanged := req.KoditTextEmbeddingProvider != nil ||
		req.KoditTextEmbeddingModel != nil ||
		req.KoditVisionEmbeddingProvider != nil ||
		req.KoditVisionEmbeddingModel != nil
	if koditEmbeddingChanged && apiServer.kodit != nil {
		go func() {
			// Use a background context — the reinit may outlive the HTTP
			// request (kodit.New can take tens of seconds when probing an
			// external embedding endpoint).
			if err := apiServer.kodit.Reinit(context.Background()); err != nil {
				log.Error().Err(err).Msg("kodit reinit failed after embedding settings change")
			}
		}()
	}

	// Return masked response with source information (same format as GET)
	envToken := os.Getenv("HF_TOKEN")
	writeResponse(rw, settings.ToResponseWithSource(settings.HuggingFaceToken, envToken), http.StatusOK)
}
