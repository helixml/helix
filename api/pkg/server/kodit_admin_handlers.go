package server

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
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

// KoditAdminQueueStats holds computed statistics about the queue.
type KoditAdminQueueStats struct {
	Total           int64                      `json:"total"`
	OldestTaskAge   string                     `json:"oldest_task_age,omitempty"`
	OldestTaskTime  *time.Time                 `json:"oldest_task_time,omitempty"`
	NewestTaskTime  *time.Time                 `json:"newest_task_time,omitempty"`
	ByOperation     map[string]int64           `json:"by_operation"`
	ByPriorityLevel map[string]int64           `json:"by_priority_level"`
}

// KoditAdminActiveTaskDTO represents a task currently being processed.
type KoditAdminActiveTaskDTO struct {
	Operation    string    `json:"operation"`
	State        string    `json:"state"`
	Message      string    `json:"message,omitempty"`
	Current      int       `json:"current"`
	Total        int       `json:"total"`
	RepositoryID int64     `json:"repository_id"`
	RepoName     string    `json:"repo_name,omitempty"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// KoditAdminQueueListResponse is the paginated list of all queue tasks.
type KoditAdminQueueListResponse struct {
	ActiveTasks []KoditAdminActiveTaskDTO `json:"active_tasks"`
	Data        []KoditAdminQueueTaskDTO  `json:"data"`
	Meta        KoditAdminPaginationMeta  `json:"meta"`
	Stats       KoditAdminQueueStats      `json:"stats"`
}

// KoditAdminQueueTaskDTO represents a task in the global queue view.
type KoditAdminQueueTaskDTO struct {
	ID           int64     `json:"id"`
	Operation    string    `json:"operation"`
	Priority     int       `json:"priority"`
	RepositoryID int64     `json:"repository_id"`
	RepoName     string    `json:"repo_name,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// KoditAdminUpdatePriorityRequest is the request body for updating task priority.
type KoditAdminUpdatePriorityRequest struct {
	Priority int `json:"priority"`
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

// helixRepoIndex provides bidirectional lookup between Kodit and Helix repositories.
// It first tries the fast path (kodit_repo_id in metadata), then falls back to
// extracting the Helix repo ID from the Kodit remote URL. When a fallback match
// is found, it writes kodit_repo_id to the Helix repo's metadata so future
// lookups use the fast path.
type helixRepoIndex struct {
	byKoditID map[int64]helixRepoRef
	byHelixID map[string]*types.GitRepository
}

func newHelixRepoIndex(helixRepos []*types.GitRepository) helixRepoIndex {
	idx := helixRepoIndex{
		byKoditID: make(map[int64]helixRepoRef, len(helixRepos)),
		byHelixID: make(map[string]*types.GitRepository, len(helixRepos)),
	}
	for _, r := range helixRepos {
		idx.byHelixID[r.ID] = r
		if r.Metadata == nil {
			continue
		}
		koditID := extractKoditRepoID(r.Metadata)
		if koditID == 0 {
			continue
		}
		idx.byKoditID[koditID] = helixRepoRef{id: r.ID, name: r.Name}
	}
	return idx
}

// lookup returns the Helix repo ref for a Kodit repo ID, falling back to
// URL-based matching. When a fallback match is found, it writes the kodit_repo_id
// into the Helix repo's metadata via the store so it sticks for next time.
func (idx helixRepoIndex) lookup(koditRepoID int64, remoteURL string, db store.Store, r *http.Request) helixRepoRef {
	if ref, ok := idx.byKoditID[koditRepoID]; ok {
		return ref
	}

	// Fallback: extract helix repo ID from the Kodit remote URL path (/git/{id})
	helixID := extractHelixRepoIDFromKoditURL(remoteURL)
	if helixID == "" {
		return helixRepoRef{}
	}

	helixRepo, ok := idx.byHelixID[helixID]
	if !ok {
		return helixRepoRef{}
	}

	// Write kodit_repo_id to metadata so we don't need the fallback next time
	if helixRepo.Metadata == nil {
		helixRepo.Metadata = make(map[string]interface{})
	}
	helixRepo.Metadata["kodit_repo_id"] = koditRepoID
	if err := db.UpdateGitRepository(r.Context(), helixRepo); err != nil {
		log.Warn().Err(err).Str("helix_repo_id", helixID).Int64("kodit_repo_id", koditRepoID).
			Msg("Failed to write kodit_repo_id to Helix repo metadata")
	} else {
		log.Info().Str("helix_repo_id", helixID).Int64("kodit_repo_id", koditRepoID).
			Msg("Linked Kodit repo to Helix repo via URL match")
	}

	ref := helixRepoRef{id: helixRepo.ID, name: helixRepo.Name}
	idx.byKoditID[koditRepoID] = ref
	return ref
}

// extractHelixRepoIDFromKoditURL extracts the Helix repo ID from a Kodit clone
// URL like http://api:apikey@api:8080/git/{helixRepoID}.
func extractHelixRepoIDFromKoditURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) >= 2 && segments[0] == "git" {
		return segments[1]
	}
	return ""
}

// sanitizeRemoteURL strips credentials from a URL for safe display.
func sanitizeRemoteURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.User = nil
	return u.String()
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
	idx := newHelixRepoIndex(helixRepos)

	// Fetch real tracking status from Kodit for each repo
	data := make([]KoditAdminRepoDTO, 0, len(repos))
	for _, repo := range repos {
		ref := idx.lookup(repo.ID(), repo.RemoteURL(), apiServer.Store, r)

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
				RemoteURL:     sanitizeRemoteURL(repo.RemoteURL()),
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
	idx := newHelixRepoIndex(helixRepos)
	ref := idx.lookup(koditRepoID, repo.RemoteURL(), apiServer.Store, r)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminRepoDetailResponse{
		Data: KoditAdminRepoDetailDTO{
			ID:   strconv.FormatInt(repo.ID(), 10),
			Type: "kodit_repository",
			Attributes: KoditAdminRepoDetailAttributes{
				RemoteURL:          sanitizeRemoteURL(repo.RemoteURL()),
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

// adminListKoditQueue lists all pending tasks in the global task queue.
// @Summary List Kodit task queue (admin)
// @Description List all pending tasks across all repositories with pagination. Admin only.
// @Tags admin
// @Produce json
// @Param page query int false "Page number (default 1)"
// @Param per_page query int false "Items per page (default 25, max 100)"
// @Success 200 {object} KoditAdminQueueListResponse
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/queue [get]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminListKoditQueue(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(KoditAdminQueueListResponse{
			Data: []KoditAdminQueueTaskDTO{},
			Meta: KoditAdminPaginationMeta{Page: 1, PerPage: 25, Total: 0, TotalPages: 0},
		})
		return
	}

	page, perPage := parsePagination(r, 1, 25, 100)
	offset := (page - 1) * perPage

	tasks, total, err := apiServer.koditService.ListAllTasks(r.Context(), perPage, offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to list Kodit task queue")
		http.Error(w, fmt.Sprintf("Failed to list task queue: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Fetch active (in-progress) tasks early so we can resolve their repo names too
	active, activeErr := apiServer.koditService.ActiveTasks(r.Context())
	if activeErr != nil {
		log.Warn().Err(activeErr).Msg("Failed to fetch active Kodit tasks")
	}

	// Build reverse index: kodit repo ID → helix repo name
	// Collect unique kodit repo IDs from task payloads and active tasks
	helixRepos, err := apiServer.Store.ListGitRepositories(r.Context(), &types.ListGitRepositoriesRequest{})
	if err != nil {
		log.Warn().Err(err).Msg("Failed to cross-reference Helix repositories for queue")
	}
	idx := newHelixRepoIndex(helixRepos)

	koditRepoIDs := make(map[int64]struct{})
	for _, t := range tasks {
		if t.RepositoryID > 0 {
			koditRepoIDs[t.RepositoryID] = struct{}{}
		}
	}
	for _, a := range active {
		if a.RepositoryID > 0 {
			koditRepoIDs[a.RepositoryID] = struct{}{}
		}
	}

	// For each unique kodit repo ID, try the fast path first (already in idx.byKoditID).
	// For those not found, fetch the repo summary to get the remote URL for fallback.
	koditRepoNames := make(map[int64]string)
	for koditID := range koditRepoIDs {
		ref := idx.byKoditID[koditID]
		if ref.name != "" {
			koditRepoNames[koditID] = ref.name
			continue
		}
		// Fallback: fetch repo summary to get remote URL
		summary, err := apiServer.koditService.RepositorySummary(r.Context(), koditID)
		if err != nil {
			continue
		}
		repo := summary.Source().Repository()
		ref = idx.lookup(koditID, repo.RemoteURL(), apiServer.Store, r)
		if ref.name != "" {
			koditRepoNames[koditID] = ref.name
		}
	}

	data := make([]KoditAdminQueueTaskDTO, 0, len(tasks))
	for _, t := range tasks {
		data = append(data, KoditAdminQueueTaskDTO{
			ID:           t.ID,
			Operation:    t.Operation,
			Priority:     t.Priority,
			RepositoryID: t.RepositoryID,
			RepoName:     koditRepoNames[t.RepositoryID],
			CreatedAt:    t.CreatedAt,
		})
	}

	totalPages := int64(math.Ceil(float64(total) / float64(perPage)))

	// Compute queue stats from all tasks (fetch full list if we don't already have it)
	allTasks := tasks
	if total > int64(len(tasks)) {
		allTasks, _, err = apiServer.koditService.ListAllTasks(r.Context(), 500, 0)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to fetch full task list for stats")
			allTasks = tasks // fall back to current page
		}
	}
	queueStats := computeQueueStats(allTasks, total)

	// Build active tasks DTOs from earlier fetch
	activeTasks := make([]KoditAdminActiveTaskDTO, 0, len(active))
	for _, a := range active {
		activeTasks = append(activeTasks, KoditAdminActiveTaskDTO{
			Operation:    a.Operation,
			State:        a.State,
			Message:      a.Message,
			Current:      a.Current,
			Total:        a.Total,
			RepositoryID: a.RepositoryID,
			RepoName:     koditRepoNames[a.RepositoryID],
			UpdatedAt:    a.UpdatedAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(KoditAdminQueueListResponse{
		ActiveTasks: activeTasks,
		Data:        data,
		Meta: KoditAdminPaginationMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
		Stats: queueStats,
	})
}

func computeQueueStats(tasks []services.KoditPendingTask, total int64) KoditAdminQueueStats {
	stats := KoditAdminQueueStats{
		Total:           total,
		ByOperation:     make(map[string]int64),
		ByPriorityLevel: make(map[string]int64),
	}

	if len(tasks) == 0 {
		return stats
	}

	var oldest, newest time.Time
	for _, t := range tasks {
		// Track oldest/newest
		if oldest.IsZero() || t.CreatedAt.Before(oldest) {
			oldest = t.CreatedAt
		}
		if newest.IsZero() || t.CreatedAt.After(newest) {
			newest = t.CreatedAt
		}

		// Count by operation (use the short name)
		parts := strings.Split(t.Operation, ".")
		shortOp := parts[len(parts)-1]
		stats.ByOperation[shortOp]++

		// Count by priority level
		var level string
		switch {
		case t.Priority >= 10000:
			level = "critical"
		case t.Priority >= 5000:
			level = "user_initiated"
		case t.Priority >= 2000:
			level = "normal"
		default:
			level = "background"
		}
		stats.ByPriorityLevel[level]++
	}

	if !oldest.IsZero() {
		stats.OldestTaskTime = &oldest
		stats.NewestTaskTime = &newest
		age := time.Since(oldest)
		stats.OldestTaskAge = formatQueueAge(age)
	}

	return stats
}

func formatQueueAge(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		h := int(d.Hours())
		m := int(d.Minutes()) % 60
		if m > 0 {
			return fmt.Sprintf("%dh %dm", h, m)
		}
		return fmt.Sprintf("%dh", h)
	}
	days := int(d.Hours()) / 24
	h := int(d.Hours()) % 24
	if h > 0 {
		return fmt.Sprintf("%dd %dh", days, h)
	}
	return fmt.Sprintf("%dd", days)
}

// adminDeleteKoditTask deletes a single task from the queue.
// @Summary Delete Kodit queue task (admin)
// @Description Delete a specific task from the Kodit task queue by ID. Admin only.
// @Tags admin
// @Produce json
// @Param taskId path int true "Task ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/queue/{taskId} [delete]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminDeleteKoditTask(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	taskID, ok := parseKoditTaskID(w, r)
	if !ok {
		return
	}

	if err := apiServer.koditService.DeleteTask(r.Context(), taskID); err != nil {
		handleKoditError(w, err, "Failed to delete task")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Task deleted successfully",
	})
}

// adminUpdateKoditTaskPriority updates the priority of a single task.
// @Summary Update Kodit queue task priority (admin)
// @Description Update the priority of a specific task in the Kodit task queue. Admin only.
// @Tags admin
// @Accept json
// @Produce json
// @Param taskId path int true "Task ID"
// @Param body body KoditAdminUpdatePriorityRequest true "New priority"
// @Success 200 {object} map[string]string
// @Failure 400 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/admin/kodit/queue/{taskId}/priority [patch]
// @Security BearerAuth
func (apiServer *HelixAPIServer) adminUpdateKoditTaskPriority(w http.ResponseWriter, r *http.Request) {
	if apiServer.koditService == nil || !apiServer.koditService.IsEnabled() {
		http.Error(w, "Kodit service is not enabled", http.StatusNotFound)
		return
	}

	taskID, ok := parseKoditTaskID(w, r)
	if !ok {
		return
	}

	var req KoditAdminUpdatePriorityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := apiServer.koditService.UpdateTaskPriority(r.Context(), taskID, req.Priority); err != nil {
		handleKoditError(w, err, "Failed to update task priority")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"message": "Task priority updated successfully",
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

func parseKoditTaskID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	vars := mux.Vars(r)
	idStr := vars["taskId"]
	if idStr == "" {
		http.Error(w, "Task ID is required", http.StatusBadRequest)
		return 0, false
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid task ID: %s", idStr), http.StatusBadRequest)
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
