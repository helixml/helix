package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/rs/zerolog/log"
)

// handleKoditError returns the appropriate HTTP status based on the error type
func handleKoditError(w http.ResponseWriter, err error, context string) {
	if services.IsKoditNotFound(err) {
		http.Error(w, fmt.Sprintf("%s: %s", context, err.Error()), http.StatusNotFound)
		return
	}
	if statusCode, ok := services.IsKoditError(err); ok {
		// For any Kodit error (except 404 which is handled above), return 502 Bad Gateway
		log.Error().Err(err).Int("kodit_status", statusCode).Msg(context)
		http.Error(w, fmt.Sprintf("%s: %s", context, err.Error()), http.StatusBadGateway)
		return
	}
	// Non-Kodit error (internal error)
	log.Error().Err(err).Msg(context)
	http.Error(w, fmt.Sprintf("%s: %s", context, err.Error()), http.StatusInternalServerError)
}

// getRepositoryEnrichments fetches code intelligence enrichments from Kodit
// @Summary Get repository enrichments
// @Description Get code intelligence enrichments for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param enrichment_type query string false "Filter by enrichment type (usage, developer, living_documentation)"
// @Param commit_sha query string false "Filter by commit SHA"
// @Success 200 {object} services.KoditEnrichmentListResponse
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Failure 502 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/enrichments [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryEnrichments(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	// Get optional enrichment type filter from query params
	enrichmentType := r.URL.Query().Get("enrichment_type")
	commitSHA := r.URL.Query().Get("commit_sha")

	// Get repository to check kodit_repo_id
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		log.Error().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata")
		http.Error(w, "Kodit repository ID not configured", http.StatusInternalServerError)
		return
	}

	// Fetch enrichments from Kodit
	enrichments, err := apiServer.koditService.GetRepositoryEnrichments(r.Context(), koditRepoID, enrichmentType, commitSHA)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Str("enrichment_type", enrichmentType).Str("commit_sha", commitSHA).Msg("Failed to fetch enrichments from Kodit")
		handleKoditError(w, err, "Failed to fetch enrichments")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enrichments)
}

// getEnrichment fetches a specific enrichment by ID from Kodit
// @Summary Get enrichment by ID
// @Description Get a specific code intelligence enrichment by ID from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param enrichmentId path string true "Enrichment ID"
// @Success 200 {object} services.KoditEnrichmentData
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Failure 502 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/enrichments/{enrichmentId} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getEnrichment(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	enrichmentID := vars["enrichmentId"]

	if repoID == "" || enrichmentID == "" {
		http.Error(w, "Repository ID and enrichment ID are required", http.StatusBadRequest)
		return
	}

	// Get repository to check kodit_repo_id (verify user has access)
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		log.Error().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata")
		http.Error(w, "Kodit repository ID not configured", http.StatusInternalServerError)
		return
	}

	// Fetch enrichment from Kodit by ID
	enrichment, err := apiServer.koditService.GetEnrichment(r.Context(), enrichmentID)
	if err != nil {
		log.Error().Err(err).Str("enrichment_id", enrichmentID).Msg("Failed to fetch enrichment from Kodit")
		handleKoditError(w, err, "Failed to fetch enrichment")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enrichment)
}

// getRepositoryKoditCommits fetches commits from Kodit
// @Summary Get repository commits from Kodit
// @Description Get commits for a repository from Kodit (used for enrichment filtering)
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param limit query int false "Limit number of commits (default 100)"
// @Success 200 {array} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Failure 502 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/kodit-commits [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryKoditCommits(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	// Get optional limit from query params (default 100)
	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get repository to check kodit_repo_id
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		log.Error().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata")
		http.Error(w, "Kodit repository ID not configured", http.StatusInternalServerError)
		return
	}

	// Fetch commits from Kodit
	commits, err := apiServer.koditService.GetRepositoryCommits(r.Context(), koditRepoID, limit)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Msg("Failed to fetch commits from Kodit")
		handleKoditError(w, err, "Failed to fetch commits")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commits)
}

// searchRepositorySnippets searches for code snippets in a repository from Kodit
// @Summary Search repository snippets
// @Description Search for code snippets in a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param query query string true "Search query"
// @Param limit query int false "Limit number of results (default 20)"
// @Param commit_sha query string false "Filter by commit SHA"
// @Success 200 {array} services.KoditSearchResult
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Failure 502 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/search-snippets [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) searchRepositorySnippets(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	// Get required query parameter
	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "Query parameter is required", http.StatusBadRequest)
		return
	}

	// Get optional commit SHA from query params
	commitSHA := r.URL.Query().Get("commit_sha")

	// Get optional limit from query params (default 20)
	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	// Get repository to check kodit_repo_id
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		log.Error().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata")
		http.Error(w, "Kodit repository ID not configured", http.StatusInternalServerError)
		return
	}

	// Search snippets from Kodit
	snippets, err := apiServer.koditService.SearchSnippets(r.Context(), koditRepoID, query, limit, commitSHA)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Str("query", query).Str("commit_sha", commitSHA).Msg("Failed to search snippets from Kodit")
		handleKoditError(w, err, "Failed to search snippets")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(snippets)
}

// getRepositoryIndexingStatus fetches indexing status from Kodit
// @Summary Get repository indexing status
// @Description Get indexing status for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} services.KoditIndexingStatus
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Failure 502 {object} types.APIError
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
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID string
	if repository.Metadata != nil {
		if id, ok := repository.Metadata["kodit_repo_id"].(string); ok {
			koditRepoID = id
		}
	}

	if koditRepoID == "" {
		log.Error().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata")
		http.Error(w, "Kodit repository ID not configured", http.StatusInternalServerError)
		return
	}

	// Fetch status from Kodit
	status, err := apiServer.koditService.GetRepositoryStatus(r.Context(), koditRepoID)
	if err != nil {
		log.Error().Err(err).Str("kodit_repo_id", koditRepoID).Msg("Failed to fetch status from Kodit")
		handleKoditError(w, err, "Failed to fetch indexing status")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}
