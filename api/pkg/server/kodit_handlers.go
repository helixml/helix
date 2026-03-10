package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit/domain/enrichment"
	"github.com/helixml/kodit/domain/repository"
	"github.com/helixml/kodit/domain/tracking"
	"github.com/rs/zerolog/log"
)

// =============================================================================
// JSON DTOs — presentation-layer types for HTTP responses
// =============================================================================

// KoditEnrichmentListResponse is the JSON envelope for a list of enrichments.
type KoditEnrichmentListResponse struct {
	Data []KoditEnrichmentDTO `json:"data"`
}

// KoditEnrichmentDTO is the JSON representation of a single enrichment.
type KoditEnrichmentDTO struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Attributes KoditEnrichmentAttributes `json:"attributes"`
	CommitSHA  string                    `json:"commit_sha,omitempty"`
}

// KoditEnrichmentAttributes holds the enrichment payload fields.
type KoditEnrichmentAttributes struct {
	Type      string    `json:"type"`
	Subtype   string    `json:"subtype,omitempty"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KoditSearchResultDTO is the JSON representation of a search result.
type KoditSearchResultDTO struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Language string `json:"language"`
	Content  string `json:"content"`
}

// KoditIndexingStatusDTO wraps status in a JSON:API envelope to match the
// frontend's expected shape: data.data.attributes.{status,message,updated_at}.
type KoditIndexingStatusDTO struct {
	Data KoditIndexingStatusData `json:"data"`
}

// KoditIndexingStatusData is the JSON:API data object for indexing status.
type KoditIndexingStatusData struct {
	Type       string                        `json:"type"`
	ID         string                        `json:"id"`
	Attributes KoditIndexingStatusAttributes `json:"attributes"`
}

// KoditIndexingStatusAttributes holds the indexing status fields.
type KoditIndexingStatusAttributes struct {
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KoditCommitDTO is the JSON representation of a commit.
type KoditCommitDTO struct {
	ID         string                `json:"id"`
	Type       string                `json:"type"`
	Attributes KoditCommitAttributes `json:"attributes"`
}

// KoditCommitAttributes holds the commit payload fields.
type KoditCommitAttributes struct {
	SHA         string    `json:"sha"`
	Message     string    `json:"message"`
	AuthoredAt  time.Time `json:"authored_at"`
	CommittedAt time.Time `json:"committed_at"`
}

// =============================================================================
// Domain → DTO conversion
// =============================================================================

const enrichmentListMaxContentLength = 500

func enrichmentToDTO(e enrichment.Enrichment) KoditEnrichmentDTO {
	return KoditEnrichmentDTO{
		Type: string(e.Type()),
		ID:   strconv.FormatInt(e.ID(), 10),
		Attributes: KoditEnrichmentAttributes{
			Type:      string(e.Type()),
			Subtype:   string(e.Subtype()),
			Content:   e.Content(),
			CreatedAt: e.CreatedAt(),
			UpdatedAt: e.UpdatedAt(),
		},
	}
}

func enrichmentListToDTO(enrichments []enrichment.Enrichment, commitSHA string) KoditEnrichmentListResponse {
	data := make([]KoditEnrichmentDTO, 0, len(enrichments))
	for _, e := range enrichments {
		dto := enrichmentToDTO(e)
		dto.CommitSHA = commitSHA
		// Truncate content in list view (use runes to avoid splitting multi-byte UTF-8)
		runes := []rune(dto.Attributes.Content)
		if len(runes) > enrichmentListMaxContentLength {
			dto.Attributes.Content = string(runes[:enrichmentListMaxContentLength]) + "..."
		}
		data = append(data, dto)
	}
	return KoditEnrichmentListResponse{Data: data}
}

func searchResultsToDTO(enrichments []enrichment.Enrichment) []KoditSearchResultDTO {
	results := make([]KoditSearchResultDTO, 0, len(enrichments))
	for _, e := range enrichments {
		results = append(results, KoditSearchResultDTO{
			ID:       strconv.FormatInt(e.ID(), 10),
			Type:     string(e.Type()),
			Language: e.Language(),
			Content:  e.Content(),
		})
	}
	return results
}

func indexingStatusToDTO(summary tracking.RepositoryStatusSummary) KoditIndexingStatusDTO {
	return KoditIndexingStatusDTO{
		Data: KoditIndexingStatusData{
			Type: "repository_status_summary",
			Attributes: KoditIndexingStatusAttributes{
				Status:    string(summary.Status()),
				Message:   summary.Message(),
				UpdatedAt: summary.UpdatedAt(),
			},
		},
	}
}

func commitsToDTO(commits []repository.Commit) []KoditCommitDTO {
	result := make([]KoditCommitDTO, 0, len(commits))
	for _, c := range commits {
		result = append(result, KoditCommitDTO{
			ID:   c.SHA(), // Frontend uses commit.id as the SHA
			Type: "commit",
			Attributes: KoditCommitAttributes{
				SHA:         c.SHA(),
				Message:     c.Message(),
				AuthoredAt:  c.AuthoredAt(),
				CommittedAt: c.CommittedAt(),
			},
		})
	}
	return result
}

// =============================================================================
// Handler helpers
// =============================================================================

// ensureKoditRepoID checks if kodit_repo_id is set in the repository metadata.
// If missing, it attempts to re-register the repository with Kodit and update the metadata.
// Returns the kodit_repo_id (int64), the repository, and whether the operation succeeded.
func (apiServer *HelixAPIServer) ensureKoditRepoID(w http.ResponseWriter, r *http.Request, repoID string) (int64, *types.GitRepository, bool) {
	repository, err := apiServer.gitRepositoryService.GetRepository(r.Context(), repoID)
	if err != nil {
		log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to get repository")
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return 0, nil, false
	}

	// Check if Kodit indexing is enabled
	if !repository.KoditIndexing {
		http.Error(w, "Kodit indexing not enabled for this repository", http.StatusNotFound)
		return 0, nil, false
	}

	// Early exit if the kodit service itself isn't running
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return 0, nil, false
	}

	// Check if kodit_repo_id is set in metadata
	var koditRepoID int64
	if repository.Metadata != nil {
		koditRepoID = extractKoditRepoID(repository.Metadata)
	}

	// If kodit_repo_id is missing, try to re-register with Kodit
	if koditRepoID == 0 {
		log.Warn().Str("repo_id", repoID).Msg("Repository has KoditIndexing=true but no kodit_repo_id in metadata, attempting re-registration")

		koditRepoID, err = apiServer.reRegisterWithKodit(r, repository)
		if err != nil {
			log.Error().Err(err).Str("repo_id", repoID).Msg("Failed to re-register repository with Kodit")
			http.Error(w, fmt.Sprintf("Kodit repository ID not configured and re-registration failed: %s", err.Error()), http.StatusInternalServerError)
			return 0, nil, false
		}

		log.Info().Str("repo_id", repoID).Int64("kodit_repo_id", koditRepoID).Msg("Successfully re-registered repository with Kodit")
	}

	return koditRepoID, repository, true
}

// extractKoditRepoID extracts the kodit_repo_id from metadata, handling both
// int64 (new) and string (legacy) and float64 (JSON number) formats.
func extractKoditRepoID(metadata map[string]interface{}) int64 {
	raw, ok := metadata["kodit_repo_id"]
	if !ok {
		return 0
	}
	switch v := raw.(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	case int:
		return int64(v)
	case string:
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			return id
		}
	case json.Number:
		if id, err := v.Int64(); err == nil {
			return id
		}
	}
	return 0
}

// reRegisterWithKodit attempts to register a repository with Kodit and update its metadata.
func (apiServer *HelixAPIServer) reRegisterWithKodit(r *http.Request, repository *types.GitRepository) (int64, error) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		return 0, fmt.Errorf("kodit service not available or not enabled")
	}

	user := getRequestUser(r)
	if user == nil {
		return 0, fmt.Errorf("user not found in request context")
	}

	apiKey, err := apiServer.getOrCreateUserAPIKey(r.Context(), user)
	if err != nil {
		return 0, fmt.Errorf("failed to get user API key: %w", err)
	}

	koditCloneURL := apiServer.gitRepositoryService.BuildAuthenticatedCloneURL(repository.ID, apiKey)

	log.Debug().
		Str("repo_id", repository.ID).
		Str("clone_url", koditCloneURL).
		Msg("Attempting to re-register repository with Kodit")

	koditRepoID, _, err := apiServer.koditService.RegisterRepository(r.Context(), koditCloneURL)
	if err != nil {
		return 0, fmt.Errorf("failed to register repository with Kodit: %w", err)
	}

	// Update repository metadata with Kodit ID
	if repository.Metadata == nil {
		repository.Metadata = make(map[string]interface{})
	}
	repository.Metadata["kodit_repo_id"] = koditRepoID

	if err := apiServer.Store.UpdateGitRepository(r.Context(), repository); err != nil {
		log.Warn().
			Err(err).
			Str("repo_id", repository.ID).
			Int64("kodit_repo_id", koditRepoID).
			Msg("Failed to update repository with Kodit ID after re-registration")
	}

	return koditRepoID, nil
}

// handleKoditError returns the appropriate HTTP status based on the error type
func handleKoditError(w http.ResponseWriter, err error, context string) {
	if errors.Is(err, services.ErrKoditNotFound) {
		http.Error(w, fmt.Sprintf("%s: %s", context, err.Error()), http.StatusNotFound)
		return
	}
	log.Error().Err(err).Msg(context)
	http.Error(w, fmt.Sprintf("%s: %s", context, err.Error()), http.StatusInternalServerError)
}

// =============================================================================
// HTTP handlers
// =============================================================================

// getRepositoryEnrichments fetches code intelligence enrichments from Kodit
// @Summary Get repository enrichments
// @Description Get code intelligence enrichments for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param enrichment_type query string false "Filter by enrichment type (usage, developer, living_documentation)"
// @Param commit_sha query string false "Filter by commit SHA"
// @Success 200 {object} KoditEnrichmentListResponse
// @Failure 400 {object} types.APIError
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

	enrichmentType := r.URL.Query().Get("enrichment_type")
	commitSHA := r.URL.Query().Get("commit_sha")

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	enrichments, err := apiServer.koditService.GetRepositoryEnrichments(r.Context(), koditRepoID, enrichmentType, commitSHA)
	if err != nil {
		log.Error().Err(err).Int64("kodit_repo_id", koditRepoID).Str("enrichment_type", enrichmentType).Str("commit_sha", commitSHA).Msg("Failed to fetch enrichments from Kodit")
		handleKoditError(w, err, "Failed to fetch enrichments")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enrichmentListToDTO(enrichments, commitSHA))
}

// getEnrichment fetches a specific enrichment by ID from Kodit.
// TODO: enrichment is fetched by global ID without verifying it belongs to this
// repo's commits. A user with access to any repo can read any enrichment by ID.
// Fix requires kodit library to expose commit/repo relationship on enrichments.
// @Summary Get enrichment by ID
// @Description Get a specific code intelligence enrichment by ID from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param enrichmentId path string true "Enrichment ID"
// @Success 200 {object} KoditEnrichmentDTO
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
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

	_, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	e, err := apiServer.koditService.GetEnrichment(r.Context(), enrichmentID)
	if err != nil {
		log.Error().Err(err).Str("enrichment_id", enrichmentID).Msg("Failed to fetch enrichment from Kodit")
		handleKoditError(w, err, "Failed to fetch enrichment")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(enrichmentToDTO(e))
}

// getRepositoryKoditCommits fetches commits from Kodit
// @Summary Get repository commits from Kodit
// @Description Get commits for a repository from Kodit (used for enrichment filtering)
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param limit query int false "Limit number of commits (default 100)"
// @Success 200 {array} KoditCommitDTO
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/kodit-commits [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryKoditCommits(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	limit := 100
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if limit > 500 {
		limit = 500
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	commits, err := apiServer.koditService.GetRepositoryCommits(r.Context(), koditRepoID, limit)
	if err != nil {
		log.Error().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to fetch commits from Kodit")
		handleKoditError(w, err, "Failed to fetch commits")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(commitsToDTO(commits))
}

// searchRepositorySnippets searches for code snippets in a repository from Kodit
// @Summary Search repository snippets
// @Description Search for code snippets in a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param query query string true "Search query"
// @Param limit query int false "Limit number of results (default 20)"
// @Success 200 {array} KoditSearchResultDTO
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/search-snippets [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) searchRepositorySnippets(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "Query parameter is required", http.StatusBadRequest)
		return
	}

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if limit > 100 {
		limit = 100
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	log.Debug().Str("query", query).Int("limit", limit).Int64("kodit_repo_id", koditRepoID).Msg("Searching snippets in Kodit")

	enrichments, err := apiServer.koditService.SearchSnippets(r.Context(), koditRepoID, query, limit)
	if err != nil {
		log.Error().Err(err).Int64("kodit_repo_id", koditRepoID).Str("query", query).Msg("Failed to search snippets from Kodit")
		handleKoditError(w, err, "Failed to search snippets")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(searchResultsToDTO(enrichments))
}

// getRepositoryIndexingStatus fetches indexing status from Kodit
// @Summary Get repository indexing status
// @Description Get indexing status for a repository from Kodit
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} KoditIndexingStatusDTO
// @Failure 400 {object} types.APIError
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

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	summary, err := apiServer.koditService.GetRepositoryStatus(r.Context(), koditRepoID)
	if err != nil {
		log.Error().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to fetch status from Kodit")
		handleKoditError(w, err, "Failed to fetch indexing status")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(indexingStatusToDTO(summary))
}

// getRepositoryWikiTree returns the wiki navigation tree for a repository
// @Summary Get repository wiki tree
// @Description Get the wiki navigation tree (titles and paths, no content) for a repository
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/wiki [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryWikiTree(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	nodes, err := apiServer.koditService.GetWikiTree(r.Context(), koditRepoID)
	if err != nil {
		handleKoditError(w, err, "Failed to fetch wiki tree")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"data": nodes})
}

// getRepositoryWikiPage returns a single wiki page by path
// @Summary Get wiki page
// @Description Get a wiki page by hierarchical path as markdown content
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param path query string true "Wiki page path (e.g. architecture/database-layer.md)"
// @Success 200 {object} services.KoditWikiPage
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/wiki-page [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) getRepositoryWikiPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	pagePath := r.URL.Query().Get("path")
	if pagePath == "" {
		http.Error(w, "path query parameter is required", http.StatusBadRequest)
		return
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	page, err := apiServer.koditService.GetWikiPage(r.Context(), koditRepoID, pagePath)
	if err != nil {
		handleKoditError(w, err, "Failed to fetch wiki page")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(page)
}

// semanticSearchRepository performs vector similarity search
func (apiServer *HelixAPIServer) semanticSearchRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	query := r.URL.Query().Get("query")
	if query == "" {
		http.Error(w, "query parameter is required", http.StatusBadRequest)
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if limit > 100 {
		limit = 100
	}

	language := r.URL.Query().Get("language")

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	results, err := apiServer.koditService.SemanticSearch(r.Context(), koditRepoID, query, limit, language)
	if err != nil {
		handleKoditError(w, err, "Failed to perform semantic search")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// keywordSearchRepository performs BM25 keyword search
func (apiServer *HelixAPIServer) keywordSearchRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	keywords := r.URL.Query().Get("keywords")
	if keywords == "" {
		http.Error(w, "keywords parameter is required", http.StatusBadRequest)
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if limit > 100 {
		limit = 100
	}

	language := r.URL.Query().Get("language")

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	results, err := apiServer.koditService.KeywordSearch(r.Context(), koditRepoID, keywords, limit, language)
	if err != nil {
		handleKoditError(w, err, "Failed to perform keyword search")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// grepRepository runs git grep against a repository
func (apiServer *HelixAPIServer) grepRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		http.Error(w, "pattern parameter is required", http.StatusBadRequest)
		return
	}

	glob := r.URL.Query().Get("glob")

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if limit > 200 {
		limit = 200
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	results, err := apiServer.koditService.GrepSearch(r.Context(), koditRepoID, pattern, glob, limit)
	if err != nil {
		handleKoditError(w, err, "Failed to perform grep search")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// listRepositoryFiles lists files matching a glob pattern
func (apiServer *HelixAPIServer) listRepositoryFiles(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	pattern := r.URL.Query().Get("pattern")
	if pattern == "" {
		pattern = "**/*"
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	entries, err := apiServer.koditService.ListFiles(r.Context(), koditRepoID, pattern)
	if err != nil {
		handleKoditError(w, err, "Failed to list files")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(entries)
}

// readRepositoryFile reads the content of a file from the repository
func (apiServer *HelixAPIServer) readRepositoryFile(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "path parameter is required", http.StatusBadRequest)
		return
	}

	var startLine, endLine int
	if s := r.URL.Query().Get("start_line"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			startLine = v
		}
	}
	if s := r.URL.Query().Get("end_line"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			endLine = v
		}
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	content, err := apiServer.koditService.ReadFile(r.Context(), koditRepoID, filePath, startLine, endLine)
	if err != nil {
		handleKoditError(w, err, "Failed to read file")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(content)
}

// rescanRepository triggers a rescan of a specific commit in Kodit
// @Summary Rescan repository commit
// @Description Trigger a rescan of a specific commit in Kodit to refresh code intelligence
// @Tags git-repositories
// @Produce json
// @Param id path string true "Repository ID"
// @Param commit_sha query string true "Commit SHA to rescan"
// @Success 200 {object} map[string]string
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git/repositories/{id}/kodit-rescan [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) rescanRepository(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	repoID := vars["id"]
	if repoID == "" {
		http.Error(w, "Repository ID is required", http.StatusBadRequest)
		return
	}

	commitSHA := r.URL.Query().Get("commit_sha")
	if commitSHA == "" {
		http.Error(w, "commit_sha query parameter is required", http.StatusBadRequest)
		return
	}

	koditRepoID, _, ok := apiServer.ensureKoditRepoID(w, r, repoID)
	if !ok {
		return
	}

	err := apiServer.koditService.RescanCommit(r.Context(), koditRepoID, commitSHA)
	if err != nil {
		log.Error().Err(err).Int64("kodit_repo_id", koditRepoID).Str("commit_sha", commitSHA).Msg("Failed to trigger rescan in Kodit")
		handleKoditError(w, err, "Failed to trigger rescan")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "accepted",
		"message":    "Rescan triggered successfully",
		"commit_sha": commitSHA,
	})
}
