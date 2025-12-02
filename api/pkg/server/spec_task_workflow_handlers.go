package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// approveImplementation - called when user approves implementation
// @Summary Approve implementation and merge to main
// @Description Approve the implementation and instruct agent to merge to main branch
// @Tags spec-tasks
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {object} types.SpecTask
// @Router /api/v1/spec-tasks/{spec_task_id}/approve-implementation [post]
// @Security BearerAuth
func (s *HelixAPIServer) approveImplementation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]

	if specTaskID == "" {
		http.Error(w, "spec_task_id is required", http.StatusBadRequest)
		return
	}

	// Get spec task
	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Get project for authorization and repo info
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get project: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Authorize - check if user has access to this project
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionUpdate); err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", project.ID).
			Msg("User not authorized to approve implementation")
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	// Verify status - allow approval from implementation or implementation_review
	if specTask.Status != types.TaskStatusImplementation && specTask.Status != types.TaskStatusImplementationReview {
		http.Error(w, fmt.Sprintf("Task must be in implementation or implementation_review status, currently: %s", specTask.Status), http.StatusBadRequest)
		return
	}

	// Update task - move straight to done when implementation is approved
	now := time.Now()
	specTask.ImplementationApprovedBy = user.ID
	specTask.ImplementationApprovedAt = &now
	specTask.Status = types.TaskStatusDone
	specTask.CompletedAt = &now

	if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Get repo info for base branch
	var baseBranch string
	if project.DefaultRepoID != "" {
		repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
		if err == nil && repo != nil {
			baseBranch = repo.DefaultBranch
		}
	}
	if baseBranch == "" {
		baseBranch = "main"
	}

	// Send merge instruction to agent via WebSocket
	go func() {
		message := services.BuildMergeInstructionPrompt(specTask.BranchName, baseBranch)
		_, err := s.sendMessageToSpecTaskAgent(context.Background(), specTask, message, "")
		if err != nil {
			log.Error().
				Err(err).
				Str("task_id", specTask.ID).
				Str("planning_session_id", specTask.PlanningSessionID).
				Msg("Failed to send merge instruction to agent via WebSocket")
		} else {
			log.Info().
				Str("task_id", specTask.ID).
				Str("branch_name", specTask.BranchName).
				Msg("Implementation approved - sent merge instruction to agent via WebSocket")
		}
	}()

	// Return updated task
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(specTask)
}

// stopAgentSession - stop the agent session for a spec task
// @Summary Stop agent session
// @Description Stop the running agent session for a spec task
// @Tags spec-tasks
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {object} types.SpecTask
// @Router /api/v1/spec-tasks/{spec_task_id}/stop-agent [post]
// @Security BearerAuth
func (s *HelixAPIServer) stopAgentSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]

	if specTaskID == "" {
		http.Error(w, "spec_task_id is required", http.StatusBadRequest)
		return
	}

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project for authorization
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get project: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Authorize - check if user has access to this project
	if err := s.authorizeUserToProject(ctx, user, project, types.ActionUpdate); err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", project.ID).
			Msg("User not authorized to stop agent session")
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	// Stop external agent if exists
	if specTask.ExternalAgentID != "" {
		// TODO: Call wolf executor to stop the agent
		log.Info().Str("external_agent_id", specTask.ExternalAgentID).Msg("Stopping external agent")
	}

	log.Info().
		Str("task_id", specTask.ID).
		Str("user_id", user.ID).
		Msg("Agent stop requested")

	// Return task
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(specTask)
}
