package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Implementation Phase Transition Handlers

// startImplementation transitions an approved spec task to implementation phase
// @Summary Start implementation phase
// @Description Transition an approved spec task to implementation, creating a feature branch
// @Tags SpecTasks
// @Produce json
// @Param taskId path string true "Task ID"
// @Success 200 {object} types.SpecTaskImplementationStartResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{taskId}/start-implementation [post]
// @Security ApiKeyAuth
func (s *HelixAPIServer) startImplementation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	taskID := vars["taskId"]

	if taskID == "" {
		http.Error(w, "task ID is required", http.StatusBadRequest)
		return
	}

	// Get spec task
	specTask, err := s.Store.GetSpecTask(ctx, taskID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Check authorization using project-level auth (handles personal + org projects)
	if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionUpdate); err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", specTask.ProjectID).
			Msg("User not authorized to start implementation")
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	// Verify spec is approved
	if specTask.Status != types.TaskStatusSpecApproved {
		http.Error(w, fmt.Sprintf("Cannot start implementation: spec task status is '%s', must be 'spec_approved'", specTask.Status), http.StatusBadRequest)
		return
	}

	// Generate feature branch name
	branchName := generateFeatureBranchName(specTask)

	// Get project repository
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get project: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	if project.DefaultRepoID == "" {
		http.Error(w, "No repository configured for this project", http.StatusBadRequest)
		return
	}

	repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get repository: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Record the branch name - agent will create it when they start implementation
	log.Info().
		Str("spec_task_id", taskID).
		Str("branch_name", branchName).
		Str("base_branch", repo.DefaultBranch).
		Msg("[Implementation] Recording feature branch for agent to create")

	// Update spec task status to implementation
	specTask.Status = types.TaskStatusImplementationQueued
	specTask.BranchName = branchName
	now := time.Now()
	specTask.StartedAt = &now

	if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Agent will receive this via their next interaction or WebSocket (when implemented)
	log.Info().
		Str("spec_task_id", taskID).
		Str("branch_name", branchName).
		Msg("[Implementation] Ready for agent to start implementation")

	// Generate instructions for the agent
	agentInstructions := fmt.Sprintf(`# Implementation Phase Started

The design has been approved! You can now begin implementation.

**Branch Information:**
- Feature Branch: %s
- Base Branch: %s
- Repository: %s

**Your Tasks:**
1. Create and checkout the feature branch: git checkout -b %s
2. Implement the features according to the approved design documents
3. Follow the implementation plan breakdown
4. Write tests for all new functionality
5. Commit your work with clear, descriptive commit messages
6. When complete, push the branch and create a pull request

**Design Documents:**
- Requirements: See spec task requirements_spec field
- Technical Design: See spec task technical_design field
- Implementation Plan: See spec task implementation_plan field

Good luck! Let me know if you need any clarification on the design.
`, branchName, repo.DefaultBranch, repo.Name, branchName)

	// Prepare response
	response := &types.SpecTaskImplementationStartResponse{
		BranchName:        branchName,
		BaseBranch:        repo.DefaultBranch,
		RepositoryID:      repo.ID,
		RepositoryName:    repo.Name,
		LocalPath:         repo.LocalPath,
		Status:            specTask.Status,
		AgentInstructions: agentInstructions,
		CreatedAt:         time.Now().Format(time.RFC3339),
	}

	// Add GitHub/GitLab PR template URL if repository has remote
	if repo.CloneURL != "" {
		// Parse remote URL to generate PR link
		var prURL string
		if strings.Contains(repo.CloneURL, "github.com") {
			// GitHub format: https://github.com/org/repo/compare/branch
			repoPath := extractRepoPath(repo.CloneURL)
			prURL = fmt.Sprintf("https://github.com/%s/compare/%s", repoPath, branchName)
		} else if strings.Contains(repo.CloneURL, "gitlab.com") {
			// GitLab format: https://gitlab.com/org/repo/-/merge_requests/new?merge_request[source_branch]=branch
			repoPath := extractRepoPath(repo.CloneURL)
			prURL = fmt.Sprintf("https://gitlab.com/%s/-/merge_requests/new?merge_request[source_branch]=%s", repoPath, branchName)
		}
		response.PRTemplateURL = prURL
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Helper functions

func generateFeatureBranchName(task *types.SpecTask) string {
	// Sanitize task name for branch name
	name := strings.ToLower(task.Name)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove special characters
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return -1
	}, name)

	// Truncate to reasonable length
	if len(name) > 50 {
		name = name[:50]
	}

	// Add task ID suffix for uniqueness
	// Use last 16 chars of task ID to get more of the random ULID portion
	// (ULID = 10 char timestamp + 16 char random, we want the random part)
	taskIDSuffix := task.ID
	if len(taskIDSuffix) > 16 {
		taskIDSuffix = taskIDSuffix[len(taskIDSuffix)-16:]
	}

	return fmt.Sprintf("feature/%s-%s", name, taskIDSuffix)
}

func extractRepoPath(cloneURL string) string {
	// Extract owner/repo from clone URL
	// Examples:
	//   https://github.com/owner/repo.git -> owner/repo
	//   git@github.com:owner/repo.git -> owner/repo
	url := strings.TrimSuffix(cloneURL, ".git")

	if strings.Contains(url, "github.com/") {
		parts := strings.Split(url, "github.com/")
		if len(parts) > 1 {
			return parts[1]
		}
	} else if strings.Contains(url, "gitlab.com/") {
		parts := strings.Split(url, "gitlab.com/")
		if len(parts) > 1 {
			return parts[1]
		}
	} else if strings.Contains(url, ":") {
		// git@github.com:owner/repo format
		parts := strings.Split(url, ":")
		if len(parts) > 1 {
			return parts[1]
		}
	}

	return ""
}
