package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/kodit/domain/repository"
	"github.com/rs/zerolog/log"
)

// =============================================================================
// Admin DTOs
// =============================================================================

// KoditAdminRepoListResponse is the paginated list of Kodit repositories.
type KoditAdminRepoListResponse struct {
	Data []KoditAdminRepoDTO      `json:"data"`
	Meta KoditAdminPaginationMeta `json:"meta"`
}

// KoditAdminPaginationMeta holds pagination metadata.
type KoditAdminPaginationMeta struct {
	Page       int   `json:"page"`
	PerPage    int   `json:"per_page"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}

// KoditAdminRepoDTO is a single repository in the list view.
type KoditAdminRepoDTO struct {
	ID         string                   `json:"id"`
	Type       string                   `json:"type"`
	Attributes KoditAdminRepoAttributes `json:"attributes"`
}

// KoditAdminRepoAttributes holds list-level fields.
type KoditAdminRepoAttributes struct {
	RemoteURL     string    `json:"remote_url"`
	Status        string    `json:"status"`
	LastError     string    `json:"last_error"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	HelixRepoID   string    `json:"helix_repo_id,omitempty"`
	HelixRepoName string    `json:"helix_repo_name,omitempty"`
}

// KoditAdminRepoDetailResponse is the detail view for a single repository.
type KoditAdminRepoDetailResponse struct {
	Data KoditAdminRepoDetailDTO `json:"data"`
}

// KoditAdminRepoDetailDTO is the detail DTO with summary and enrichment counts.
type KoditAdminRepoDetailDTO struct {
	ID         string                         `json:"id"`
	Type       string                         `json:"type"`
	Attributes KoditAdminRepoDetailAttributes `json:"attributes"`
}

// KoditAdminRepoDetailAttributes holds detail-level fields.
type KoditAdminRepoDetailAttributes struct {
	RemoteURL       string    `json:"remote_url"`
	Status          string    `json:"status"`
	LastError       string    `json:"last_error"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DefaultBranch   string    `json:"default_branch"`
	BranchCount     int       `json:"branch_count"`
	TagCount        int       `json:"tag_count"`
	CommitCount     int       `json:"commit_count"`
	EnrichmentCount int64     `json:"enrichment_count"`
	IndexingStatus  string    `json:"indexing_status"`
	IndexingMessage string    `json:"indexing_message"`
	HelixRepoID     string    `json:"helix_repo_id,omitempty"`
	HelixRepoName   string    `json:"helix_repo_name,omitempty"`
}

// KoditAdminBatchRequest is the request body for batch operations.
type KoditAdminBatchRequest struct {
	IDs []int64 `json:"ids"`
}

// KoditAdminBatchResponse reports results of a batch operation.
type KoditAdminBatchResponse struct {
	Succeeded []int64            `json:"succeeded"`
	Failed    []KoditBatchError  `json:"failed"`
}

// KoditBatchError pairs a repository ID with its error message.
type KoditBatchError struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// =============================================================================
// Domain → DTO conversion
// =============================================================================

func repoToAdminDTO(repo repository.Repository, helixMap map[int64]helixRepoRef) KoditAdminRepoDTO {
	src := repository.NewSource(repo)
	ref := helixMap[repo.ID()]
	return KoditAdminRepoDTO{
		ID:   strconv.FormatInt(repo.ID(), 10),
		Type: "kodit_repository",
		Attributes: KoditAdminRepoAttributes{
			RemoteURL:     repo.RemoteURL(),
			Status:        src.Status().String(),
			LastError:     src.LastError(),
			CreatedAt:     repo.CreatedAt(),
			UpdatedAt:     repo.UpdatedAt(),
			HelixRepoID:   ref.id,
			HelixRepoName: ref.name,
		},
	}
}

// helixRepoRef is a lightweight cross-reference to a Helix GitRepository.
type helixRepoRef struct {
	id   string
	name string
}

// buildHelixRepoMap creates a map from kodit_repo_id → helix repo reference.
func buildHelixRepoMap(helixRepos []*types.GitRepository) map[int64]helixRepoRef {
	m := make(map[int64]helixRepoRef, len(helixRepos))
	for _, r := range helixRepos {
		if r.Metadata == nil {
			continue
		}
		koditID := extractKoditRepoID(r.Metadata)
		if koditID == 0 {
			continue
		}
		m[koditID] = helixRepoRef{id: r.ID, name: r.Name}
	}
	return m
}

// =============================================================================
// Route registration
// =============================================================================

func (apiServer *HelixAPIServer) registerKoditAdminRoutes(adminRouter *mux.Router) {
	adminRouter.HandleFunc("/admin/kodit/repositories", apiServer.adminListKoditRepositories).Methods(http.MethodGet)
	adminRouter.HandleFunc("/admin/kodit/repositories/{koditRepoId}", apiServer.adminGetKoditRepository).Methods(http.MethodGet)
	adminRouter.HandleFunc("/admin/kodit/repositories/{koditRepoId}/sync", apiServer.adminSyncKoditRepository).Methods(http.MethodPost)
	adminRouter.HandleFunc("/admin/kodit/repositories/{koditRepoId}/rescan", apiServer.adminRescanKoditRepository).Methods(http.MethodPost)
	adminRouter.HandleFunc("/admin/kodit/repositories/{koditRepoId}", apiServer.adminDeleteKoditRepository).Methods(http.MethodDelete)
	adminRouter.HandleFunc("/admin/kodit/repositories/batch/delete", apiServer.adminBatchDeleteKoditRepositories).Methods(http.MethodPost)
	adminRouter.HandleFunc("/admin/kodit/repositories/batch/rescan", apiServer.adminBatchRescanKoditRepositories).Methods(http.MethodPost)
}

// =============================================================================
// Handlers
// =============================================================================

// adminListKoditRepositories lists all Kodit repositories with pagination.
// @Summary List Kodit repositories (admin)
// @Description List all Kodit-indexed repositories with pagination. Admin only.
// @Tags admin
// @Produce json
// @Param page query int false "Page number (default 1)"
// @Param per_page query int false "Items per page (default 25, max 100)"
// @Success 200 {object} KoditAdminRepoListResponse
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminListKoditRepositories(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	page, perPage := parsePagination(r, 1, 25, 100)
	offset := (page - 1) * perPage

	repos, total, err := apiServer.koditService.ListRepositories(r.Context(), perPage, offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list Kodit repositories")
		http.Error(w, fmt.Sprintf("Failed to list repositories: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Cross-reference with Helix GitRepository table
	helixRepos, err := apiServer.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to cross-reference Helix repositories")
	}
	helixMap := buildHelixRepoMap(helixRepos)

	data := make([]KoditAdminRepoDTO, 0, len(repos))
	for _, repo := range repos {
		data = append(data, repoToAdminDTO(repo, helixMap))
	}

	totalPages := int64(math.Ceil(float64(total) / float64(perPage)))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminRepoListResponse{
		Data: data,
		Meta: KoditAdminPaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}

// adminGetKoditRepository returns detail for a single Kodit repository.
// @Summary Get Kodit repository detail (admin)
// @Description Get detailed information about a Kodit repository including summary stats. Admin only.
// @Tags admin
// @Produce json
// @Param koditRepoId path int true "Kodit Repository ID"
// @Success 200 {object} KoditAdminRepoDetailResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/{koditRepoId} [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminGetKoditRepository(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	koditRepoID, ok := parseKoditRepoID(w, r)
	if !ok {
		return
	}

	summary, err := apiServer.koditService.RepositorySummary(r.Context(), koditRepoID)
	if err != nil {
		handleKoditError(w, err, "Failed to get repository summary")
		return
	}

	enrichmentCount, err := apiServer.koditService.EnrichmentCount(r.Context(), koditRepoID)
	if err != nil {
		log.Warn().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to get enrichment count")
	}

	trackingStatus, err := apiServer.koditService.GetRepositoryStatus(r.Context(), koditRepoID)
	if err != nil {
		log.Warn().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to get tracking status")
	}

	// Cross-reference
	helixRepos, _ := apiServer.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{})
	helixMap := buildHelixRepoMap(helixRepos)
	ref := helixMap[koditRepoID]

	src := summary.Source()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminRepoDetailResponse{
		Data: KoditAdminRepoDetailDTO{
			ID:   strconv.FormatInt(src.ID(), 10),
			Type: "kodit_repository",
			Attributes: KoditAdminRepoDetailAttributes{
				RemoteURL:       src.RemoteURL(),
				Status:          src.Status().String(),
				LastError:       src.LastError(),
				CreatedAt:       src.Repository().CreatedAt(),
				UpdatedAt:       src.Repository().UpdatedAt(),
				DefaultBranch:   summary.DefaultBranch(),
				BranchCount:     summary.BranchCount(),
				TagCount:        summary.TagCount(),
				CommitCount:     summary.CommitCount(),
				EnrichmentCount: enrichmentCount,
				IndexingStatus:  string(trackingStatus.Status()),
				IndexingMessage: trackingStatus.Message(),
				HelixRepoID:     ref.id,
				HelixRepoName:   ref.name,
			},
		},
	})
}

// adminSyncKoditRepository triggers a full sync for a Kodit repository.
// @Summary Sync Kodit repository (admin)
// @Description Trigger a full sync (git fetch + branch scan + re-index) for a Kodit repository. Admin only.
// @Tags admin
// @Produce json
// @Param koditRepoId path int true "Kodit Repository ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/{koditRepoId}/sync [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminSyncKoditRepository(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	koditRepoID, ok := parseKoditRepoID(w, r)
	if !ok {
		return
	}

	if err := apiServer.koditService.SyncRepository(r.Context(), koditRepoID); err != nil {
		handleKoditError(w, err, "Failed to sync repository")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Sync triggered successfully",
	})
}

// adminRescanKoditRepository triggers a rescan of the HEAD commit.
// @Summary Rescan Kodit repository HEAD (admin)
// @Description Trigger a rescan of the HEAD commit for a Kodit repository. Admin only.
// @Tags admin
// @Produce json
// @Param koditRepoId path int true "Kodit Repository ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/{koditRepoId}/rescan [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminRescanKoditRepository(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	koditRepoID, ok := parseKoditRepoID(w, r)
	if !ok {
		return
	}

	// Get latest commit to rescan
	commits, err := apiServer.koditService.GetRepositoryCommits(r.Context(), koditRepoID, 1)
	if err != nil {
		handleKoditError(w, err, "Failed to get commits for rescan")
		return
	}
	if len(commits) == 0 {
		http.Error(w, "No commits found for this repository", http.StatusNotFound)
		return
	}

	commitSHA := commits[0].SHA()
	if err := apiServer.koditService.RescanCommit(r.Context(), koditRepoID, commitSHA); err != nil {
		handleKoditError(w, err, "Failed to rescan commit")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":     "accepted",
		"message":    "Rescan triggered successfully",
		"commit_sha": commitSHA,
	})
}

// adminDeleteKoditRepository deletes a Kodit repository.
// @Summary Delete Kodit repository (admin)
// @Description Queue a Kodit repository for deletion. Admin only.
// @Tags admin
// @Produce json
// @Param koditRepoId path int true "Kodit Repository ID"
// @Success 200 {object} map[string]string
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/{koditRepoId} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminDeleteKoditRepository(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	koditRepoID, ok := parseKoditRepoID(w, r)
	if !ok {
		return
	}

	if err := apiServer.koditService.DeleteRepository(r.Context(), koditRepoID); err != nil {
		handleKoditError(w, err, "Failed to delete repository")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "accepted",
		"message": "Delete queued successfully",
	})
}

// adminBatchDeleteKoditRepositories deletes multiple Kodit repositories.
// @Summary Batch delete Kodit repositories (admin)
// @Description Queue multiple Kodit repositories for deletion. Admin only.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body KoditAdminBatchRequest true "Repository IDs to delete"
// @Success 200 {object} KoditAdminBatchResponse
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/batch/delete [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminBatchDeleteKoditRepositories(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	var req KoditAdminBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "No repository IDs provided", http.StatusBadRequest)
		return
	}

	resp := KoditAdminBatchResponse{}
	for _, id := range req.IDs {
		if err := apiServer.koditService.DeleteRepository(r.Context(), id); err != nil {
			resp.Failed = append(resp.Failed, KoditBatchError{ID: id, Message: err.Error()})
		} else {
			resp.Succeeded = append(resp.Succeeded, id)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// adminBatchRescanKoditRepositories rescans the HEAD commit of multiple Kodit repositories.
// @Summary Batch rescan Kodit repositories (admin)
// @Description Trigger a HEAD commit rescan for multiple Kodit repositories. Admin only.
// @Tags admin
// @Accept json
// @Produce json
// @Param body body KoditAdminBatchRequest true "Repository IDs to rescan"
// @Success 200 {object} KoditAdminBatchResponse
// @Failure 400 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/batch/rescan [post]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminBatchRescanKoditRepositories(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	var req KoditAdminBatchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if len(req.IDs) == 0 {
		http.Error(w, "No repository IDs provided", http.StatusBadRequest)
		return
	}

	resp := KoditAdminBatchResponse{}
	for _, id := range req.IDs {
		commits, err := apiServer.koditService.GetRepositoryCommits(r.Context(), id, 1)
		if err != nil {
			resp.Failed = append(resp.Failed, KoditBatchError{ID: id, Message: err.Error()})
			continue
		}
		if len(commits) == 0 {
			resp.Failed = append(resp.Failed, KoditBatchError{ID: id, Message: "no commits found"})
			continue
		}
		if err := apiServer.koditService.RescanCommit(r.Context(), id, commits[0].SHA()); err != nil {
			resp.Failed = append(resp.Failed, KoditBatchError{ID: id, Message: err.Error()})
		} else {
			resp.Succeeded = append(resp.Succeeded, id)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// =============================================================================
// Helpers
// =============================================================================

func parseKoditRepoID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	vars := mux.Vars(r)
	idStr := vars["koditRepoId"]
	if idStr == "" {
		http.Error(w, "Kodit repository ID is required", http.StatusBadRequest)
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid Kodit repository ID: %s", idStr), http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func parsePagination(r *http.Request, defaultPage, defaultPerPage, maxPerPage int) (int, int) {
	page := defaultPage
	if pageStr := r.URL.Query().Get("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	perPage := defaultPerPage
	if ppStr := r.URL.Query().Get("per_page"); ppStr != "" {
		if pp, err := strconv.Atoi(ppStr); err == nil && pp > 0 {
			perPage = pp
		}
	}
	if perPage > maxPerPage {
		perPage = maxPerPage
	}

	return page, perPage
}
