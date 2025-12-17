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

	if project.DefaultRepoID == "" {
		http.Error(w, "Default repository not set for project", http.StatusBadRequest)
		return
	}

	repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		writeErrResponse(w, fmt.Errorf("failed to get default repository: %w", err), http.StatusInternalServerError)
		return
	}

	if repo.DefaultBranch == "" {
		writeErrResponse(w, fmt.Errorf("default branch not set for repository"), http.StatusInternalServerError)
		return
	}

	now := time.Now()
	specTask.ImplementationApprovedBy = user.ID
	specTask.ImplementationApprovedAt = &now

	// If repo is external, move to pull_request status (awaiting merge in external system)
	// For internal repos, move to done and instruct agent to merge
	switch {
	case repo.AzureDevOps != nil:
		// External repo: move to pull_request status, await merge via polling
		specTask.Status = types.TaskStatusPullRequest

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Send message to agent to push a commit (triggers PR creation)
		// The git handler will create the PR when it receives a push in pull_request status
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			message := `Your implementation has been approved! Please push your changes now to open a Pull Request.

If you have uncommitted changes, commit them first. If all changes are already committed, you can push an empty commit:
git commit --allow-empty -m "chore: open pull request for review"
git push origin ` + specTask.BranchName + `

This will open a Pull Request in Azure DevOps for code review.`

			_, err := s.sendMessageToSpecTaskAgent(context.Background(), specTask, message, "")
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", specTask.ID).
					Str("planning_session_id", specTask.PlanningSessionID).
					Msg("Failed to send PR instruction to agent via WebSocket")
			} else {
				log.Info().
					Str("task_id", specTask.ID).
					Str("branch_name", specTask.BranchName).
					Msg("Implementation approved - sent PR instruction to agent via WebSocket")
			}
		}()

		// Re-fetch to get the latest PullRequestID (may have been set by concurrent push)
		updatedTask, err := s.Store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get updated spec task: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Construct PR URL for ADO repos
		if updatedTask.PullRequestID != "" {
			updatedTask.PullRequestURL = fmt.Sprintf("%s/pullrequest/%s", repo.ExternalURL, updatedTask.PullRequestID)
		}

		writeResponse(w, updatedTask, http.StatusOK)
		return
	default:
		// Internal repo: move straight to done
		specTask.Status = types.TaskStatusDone
		specTask.CompletedAt = &now
	}

	// Updating spec task
	if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Send merge instruction to agent via WebSocket
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		message := services.BuildMergeInstructionPrompt(specTask.BranchName, repo.DefaultBranch)
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
	writeResponse(w, specTask, http.StatusOK)
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
