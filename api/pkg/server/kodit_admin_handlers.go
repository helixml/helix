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
// Status and message come from Kodit's tracking service (the real indexing
// pipeline state), not from any derived/interpreted value.
type KoditAdminRepoAttributes struct {
	RemoteURL     string    `json:"remote_url"`
	Status        string    `json:"status"`
	StatusMessage string    `json:"status_message"`
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
// Status and message come directly from Kodit's tracking service.
type KoditAdminRepoDetailAttributes struct {
	RemoteURL       string    `json:"remote_url"`
	Status          string    `json:"status"`
	StatusMessage   string    `json:"status_message"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	DefaultBranch   string    `json:"default_branch"`
	BranchCount     int       `json:"branch_count"`
	TagCount        int       `json:"tag_count"`
	CommitCount     int       `json:"commit_count"`
	EnrichmentCount int64     `json:"enrichment_count"`
	HelixRepoID     string    `json:"helix_repo_id,omitempty"`
	HelixRepoName   string    `json:"helix_repo_name,omitempty"`

	// Latest commit tracked by Kodit
	LatestCommitSHA    string `json:"latest_commit_sha,omitempty"`
	LatestCommitMsg    string `json:"latest_commit_message,omitempty"`
	LatestCommitAuthor string `json:"latest_commit_author,omitempty"`
	LatestCommitDate   string `json:"latest_commit_date,omitempty"`

	// Last time Kodit scanned this repository
	LastScannedAt string `json:"last_scanned_at,omitempty"`
}

// KoditAdminStatsResponse holds aggregate system statistics.
type KoditAdminStatsResponse struct {
	Repositories int64 `json:"repositories"`
	Enrichments  int64 `json:"enrichments"`
	Commits      int64 `json:"commits"`
	PendingTasks int64 `json:"pending_tasks"`
}

// KoditAdminRepositoryTasksResponse holds tracking statuses and pending tasks for a repository.
type KoditAdminRepositoryTasksResponse struct {
	Statuses     []KoditAdminTaskStatusDTO  `json:"statuses"`
	PendingTasks []KoditAdminPendingTaskDTO `json:"pending_tasks"`
}

// KoditAdminTaskStatusDTO represents the status of a tracked operation.
type KoditAdminTaskStatusDTO struct {
	Operation string    `json:"operation"`
	State     string    `json:"state"`
	Message   string    `json:"message,omitempty"`
	Error     string    `json:"error,omitempty"`
	Current   int       `json:"current"`
	Total     int       `json:"total"`
	UpdatedAt time.Time `json:"updated_at"`
}

// KoditAdminPendingTaskDTO represents a queued task waiting to be processed.
type KoditAdminPendingTaskDTO struct {
	ID        int64     `json:"id"`
	Operation string    `json:"operation"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

// KoditAdminBatchRequest is the request body for batch operations.
type KoditAdminBatchRequest struct {
	IDs []int64 `json:"ids"`
}

// KoditAdminBatchResponse reports results of a batch operation.
type KoditAdminBatchResponse struct {
	Succeeded []int64           `json:"succeeded"`
	Failed    []KoditBatchError `json:"failed"`
}

// KoditBatchError pairs a repository ID with its error message.
type KoditBatchError struct {
	ID      int64  `json:"id"`
	Message string `json:"message"`
}

// =============================================================================
// Domain → DTO conversion
// =============================================================================

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
		// Return empty list instead of 404 so the admin page renders
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KoditAdminRepoListResponse{
			Data: []KoditAdminRepoDTO{},
			Meta: KoditAdminPaginationMeta{Page: 1, PerPage: 25, Total: 0, TotalPages: 0},
		})
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

	// Fetch real tracking status from Kodit for each repo
	data := make([]KoditAdminRepoDTO, 0, len(repos))
	for _, repo := range repos {
		ref := helixMap[repo.ID()]

		var status, statusMessage string
		trackingStatus, err := apiServer.koditService.GetRepositoryStatus(r.Context(), repo.ID())
		if err != nil {
			log.Warn().Err(err).Int64("kodit_repo_id", repo.ID()).Msg("Failed to get tracking status for list")
		} else {
			status = string(trackingStatus.Status())
			statusMessage = trackingStatus.Message()
		}

		data = append(data, KoditAdminRepoDTO{
			ID:   strconv.FormatInt(repo.ID(), 10),
			Type: "kodit_repository",
			Attributes: KoditAdminRepoAttributes{
				RemoteURL:     repo.RemoteURL(),
				Status:        status,
				StatusMessage: statusMessage,
				CreatedAt:     repo.CreatedAt(),
				UpdatedAt:     repo.UpdatedAt(),
				HelixRepoID:   ref.id,
				HelixRepoName: ref.name,
			},
		})
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

	var status, statusMessage string
	trackingStatus, err := apiServer.koditService.GetRepositoryStatus(r.Context(), koditRepoID)
	if err != nil {
		log.Warn().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to get tracking status")
	} else {
		status = string(trackingStatus.Status())
		statusMessage = trackingStatus.Message()
	}

	// Latest commit
	var latestSHA, latestMsg, latestAuthor, latestDate string
	commits, err := apiServer.koditService.GetRepositoryCommits(r.Context(), koditRepoID, 1)
	if err != nil {
		log.Warn().Err(err).Int64("kodit_repo_id", koditRepoID).Msg("Failed to get latest commit")
	} else if len(commits) > 0 {
		c := commits[0]
		latestSHA = c.SHA()
		latestMsg = c.ShortMessage()
		latestAuthor = c.Author().Name()
		if !c.AuthoredAt().IsZero() {
			latestDate = c.AuthoredAt().Format(time.RFC3339)
		}
	}

	// Last scanned time
	repo := summary.Source().Repository()
	var lastScanned string
	if !repo.LastScannedAt().IsZero() {
		lastScanned = repo.LastScannedAt().Format(time.RFC3339)
	}

	// Cross-reference
	helixRepos, _ := apiServer.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{})
	helixMap := buildHelixRepoMap(helixRepos)
	ref := helixMap[koditRepoID]

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminRepoDetailResponse{
		Data: KoditAdminRepoDetailDTO{
			ID:   strconv.FormatInt(repo.ID(), 10),
			Type: "kodit_repository",
			Attributes: KoditAdminRepoDetailAttributes{
				RemoteURL:          repo.RemoteURL(),
				Status:             status,
				StatusMessage:      statusMessage,
				CreatedAt:          repo.CreatedAt(),
				UpdatedAt:          repo.UpdatedAt(),
				DefaultBranch:      summary.DefaultBranch(),
				BranchCount:        summary.BranchCount(),
				TagCount:           summary.TagCount(),
				CommitCount:        summary.CommitCount(),
				EnrichmentCount:    enrichmentCount,
				HelixRepoID:        ref.id,
				HelixRepoName:      ref.name,
				LatestCommitSHA:    latestSHA,
				LatestCommitMsg:    latestMsg,
				LatestCommitAuthor: latestAuthor,
				LatestCommitDate:   latestDate,
				LastScannedAt:      lastScanned,
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

// adminGetKoditRepositoryTasks returns tracking statuses and pending tasks for a repository.
// @Summary Get Kodit repository tasks (admin)
// @Description Returns tracking statuses and pending queue tasks for a Kodit repository. Admin only.
// @Tags admin
// @Produce json
// @Param koditRepoId path int true "Kodit Repository ID"
// @Success 200 {object} KoditAdminRepositoryTasksResponse
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/repositories/{koditRepoId}/tasks [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminGetKoditRepositoryTasks(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KoditAdminRepositoryTasksResponse{
			Statuses:     []KoditAdminTaskStatusDTO{},
			PendingTasks: []KoditAdminPendingTaskDTO{},
		})
		return
	}

	koditRepoID, ok := parseKoditRepoID(w, r)
	if !ok {
		return
	}

	tasks, err := apiServer.koditService.RepositoryTasks(r.Context(), koditRepoID)
	if err != nil {
		handleKoditError(w, err, "Failed to get repository tasks")
		return
	}

	statuses := make([]KoditAdminTaskStatusDTO, 0, len(tasks.Statuses))
	for _, s := range tasks.Statuses {
		statuses = append(statuses, KoditAdminTaskStatusDTO{
			Operation: s.Operation,
			State:     s.State,
			Message:   s.Message,
			Error:     s.Error,
			Current:   s.Current,
			Total:     s.Total,
			UpdatedAt: s.UpdatedAt,
		})
	}

	pending := make([]KoditAdminPendingTaskDTO, 0, len(tasks.PendingTasks))
	for _, t := range tasks.PendingTasks {
		pending = append(pending, KoditAdminPendingTaskDTO{
			ID:        t.ID,
			Operation: t.Operation,
			Priority:  t.Priority,
			CreatedAt: t.CreatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminRepositoryTasksResponse{
		Statuses:     statuses,
		PendingTasks: pending,
	})
}

// adminGetKoditStats returns aggregate system statistics for Kodit.
// @Summary Get Kodit system stats (admin)
// @Description Returns aggregate counts: repositories, enrichments, commits, pending tasks. Admin only.
// @Tags admin
// @Produce json
// @Success 200 {object} KoditAdminStatsResponse
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/stats [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminGetKoditStats(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KoditAdminStatsResponse{})
		return
	}

	stats, err := apiServer.koditService.SystemStats(r.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to get Kodit system stats")
		http.Error(w, fmt.Sprintf("Failed to get system stats: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminStatsResponse{
		Repositories: stats.Repositories,
		Enrichments:  stats.Enrichments,
		Commits:      stats.Commits,
		PendingTasks: stats.PendingTasks,
	})
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
