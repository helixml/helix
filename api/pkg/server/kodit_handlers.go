package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/rs/zerolog/log"
)

// getRepositoryEnrichments fetches code intelligence enrichments from Kodit
// @Summary Get repository enrichments
// @Description Get code intelligence enrichments for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} services.KoditEnrichmentListResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/enrichments [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryEnrichments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	// Get repository to check kodit_repo_id
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Repository not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Check if repository has Kodit indexing enabled
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		// Repository not indexed by Kodit
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services.KoditEnrichmentListResponse{
			Data: []services.KoditEnrichmentData{},
		})
		return
	}

	// Fetch enrichments from Kodit
	enrichments, err := apiServer.koditService.GetRepositoryEnrichments(r.Context(), koditRepoID)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Msg("Failed to fetch enrichments from Kodit")
		http.Error(w, fmt.Sprintf("Failed to fetch enrichments: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enrichments)
}

// getRepositoryIndexingStatus fetches indexing status from Kodit
// @Summary Get repository indexing status
// @Description Get indexing status for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/kodit-status [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryIndexingStatus(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	// Get repository to check kodit_repo_id
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Repository not found: %s", err.Error()), http.StatusNotFound)
		return
	}

	// Check if repository has Kodit indexing enabled
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		// Repository not indexed by Kodit
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"message": "Kodit indexing not enabled for this repository",
		})
		return
	}

	// Fetch status from Kodit
	status, err := apiServer.koditService.GetRepositoryStatus(r.Context(), koditRepoID)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Msg("Failed to fetch status from Kodit")
		http.Error(w, fmt.Sprintf("Failed to fetch indexing status: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
