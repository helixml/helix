package server

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// listProjectLabels godoc
// @Summary List all labels used in a project
// @Description Returns a sorted list of unique labels across all spec tasks in a project
// @Tags    spec-driven-tasks
// @Produce json
// @Param   projectId path string true "Project ID"
// @Success 200 {array} string
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/projects/{projectId}/labels [get]
func (s *HelixAPIServer) listProjectLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	projectID := vars["projectId"]

	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.authorizeUserToProjectByID(ctx, user, projectID, types.ActionList); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	labels, err := s.Store.ListProjectLabels(ctx, projectID)
	if err != nil {
		log.Error().Err(err).Str("project_id", projectID).Msg("Failed to list project labels")
		http.Error(w, fmt.Sprintf("failed to list labels: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(labels)
}

type addLabelRequest struct {
	Label string `json:"label"`
}

// addSpecTaskLabel godoc
// @Summary Add a label to a spec task
// @Description Adds a label to a spec task (idempotent - no error if label already exists)
// @Tags    spec-driven-tasks
// @Accept  json
// @Produce json
// @Param   taskId path string true "Task ID"
// @Param   request body addLabelRequest true "Label to add"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/labels [post]
func (s *HelixAPIServer) addSpecTaskLabel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req addLabelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}

	if err := s.Store.AddSpecTaskLabel(ctx, taskID, req.Label); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Str("label", req.Label).Msg("Failed to add label")
		http.Error(w, fmt.Sprintf("failed to add label: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated task
	updated, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "failed to get updated task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}

// removeSpecTaskLabel godoc
// @Summary Remove a label from a spec task
// @Description Removes a label from a spec task (no-op if label does not exist)
// @Tags    spec-driven-tasks
// @Produce json
// @Param   taskId path string true "Task ID"
// @Param   label path string true "Label to remove"
// @Success 200 {object} types.SpecTask
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router  /api/v1/spec-tasks/{taskId}/labels/{label} [delete]
func (s *HelixAPIServer) removeSpecTaskLabel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	vars := mux.Vars(r)
	taskID := vars["taskId"]
	label := vars["label"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}
	if label == "" {
		http.Error(w, "label is required", http.StatusBadRequest)
		return
	}

	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	task, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	if err := s.authorizeUserToProjectByID(ctx, user, task.ProjectID, types.ActionUpdate); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	if err := s.Store.RemoveSpecTaskLabel(ctx, taskID, label); err != nil {
		log.Error().Err(err).Str("task_id", taskID).Str("label", label).Msg("Failed to remove label")
		http.Error(w, fmt.Sprintf("failed to remove label: %v", err), http.StatusInternalServerError)
		return
	}

	// Return the updated task
	updated, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, "failed to get updated task", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updated)
}
