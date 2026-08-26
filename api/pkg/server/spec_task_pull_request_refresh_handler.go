package server

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// refreshSpecTaskPullRequest godoc
// @Summary Refresh a spec task's pull request status
// @Description Immediately synchronize pull request and CI status for a task being actively viewed. Requests are coalesced to one poll per task every 30 seconds.
// @Tags spec-tasks
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 204
// @Failure 401 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 502 {object} types.APIError
// @Failure 503 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/refresh-pull-request [post]
// @Security BearerAuth
func (s *HelixAPIServer) refreshSpecTaskPullRequest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	taskID := mux.Vars(r)["taskId"]
	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionGet); err != nil {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}
	if s.specTaskOrchestrator == nil {
		http.Error(w, "spec task orchestrator unavailable", http.StatusServiceUnavailable)
		return
	}
	if err := s.specTaskOrchestrator.RefreshPullRequestStatus(ctx, taskID); err != nil {
		log.Warn().Err(err).Str("task_id", taskID).Msg("Failed to refresh pull request status")
		http.Error(w, "failed to refresh pull request status", http.StatusBadGateway)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
