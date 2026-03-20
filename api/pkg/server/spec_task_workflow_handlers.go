package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/helixml/helix/api/pkg/prompts"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
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

	// If repo is external, move to pull_request status (awaiting merge in external system)
	// For internal repos, try merge first - only record approval if merge succeeds
	if s.shouldOpenPullRequest(repo) {
		// External repo: record approval and move to pull_request status, await merge via polling
		specTask.ImplementationApprovedBy = user.ID
		specTask.ImplementationApprovedAt = &now
		specTask.Status = types.TaskStatusPullRequest
		specTask.StatusUpdatedAt = &now

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Check if branch already has commits - if so, create PR immediately
		hasCommits, err := s.branchHasCommitsAhead(ctx, repo.LocalPath, specTask.BranchName, repo.DefaultBranch)
		if err != nil {
			log.Warn().Err(err).Str("task_id", specTask.ID).Msg("Failed to check if branch has commits ahead - will wait for agent push")
		} else if hasCommits {
			log.Info().Str("task_id", specTask.ID).Str("branch", specTask.BranchName).Msg("Branch has commits ahead - creating PR immediately")
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				if err := s.ensurePullRequestForTask(context.Background(), repo, specTask); err != nil {
					log.Error().Err(err).Str("task_id", specTask.ID).Msg("Failed to auto-create PR on approval")
				}
			}()
		}

		// Always send message to agent to commit and push any remaining uncommitted changes
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			message, err := prompts.ImplementationApprovedPushInstruction(specTask.BranchName)
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", specTask.ID).
					Str("planning_session_id", specTask.PlanningSessionID).
					Msg("Failed to generate push instruction for agent")
				return
			}

			_, err = s.sendMessageToSpecTaskAgent(context.Background(), specTask, message, "")
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", specTask.ID).
					Str("planning_session_id", specTask.PlanningSessionID).
					Msg("Failed to send push instruction to agent via WebSocket")
			} else {
				log.Info().
					Str("task_id", specTask.ID).
					Str("branch_name", specTask.BranchName).
					Msg("Implementation approved - sent push instruction to agent via WebSocket")
			}
		}()

		// Re-fetch to get the latest PullRequestID (may have been set by concurrent push)
		updatedTask, err := s.Store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to get updated spec task: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Construct PR URL for ADO repos
		updatedTask.PullRequestURL = services.GetPullRequestURL(repo, updatedTask.PullRequestID)

		writeResponse(w, updatedTask, http.StatusOK)
		return
	}

	// Internal repo or external repo with no PRs automation implemented
	// Server-side merge: agent can't push to main due to branch restrictions

	// For external repos, acquire lock and sync before merge.
	// The lock serializes git operations to prevent race conditions.
	var oldDefaultBranchRef string
	if repo.IsExternal && repo.ExternalURL != "" {
		// Acquire repo lock for the entire merge flow (sync → merge → push)
		lock := s.gitRepositoryService.GetRepoLock(repo.ID)
		lock.Lock()
		defer lock.Unlock()

		if err := s.gitRepositoryService.SyncAllBranches(ctx, repo.ID, true); err != nil {
			log.Warn().
				Err(err).
				Str("task_id", specTask.ID).
				Str("repo_id", repo.ID).
				Msg("Failed to sync from upstream before merge - continuing with local state")
		}
		// Capture ref before merge for rollback
		oldDefaultBranchRef, _ = services.GetBranchCommitID(ctx, repo.LocalPath, repo.DefaultBranch)
	}

	// Try fast-forward merge of feature branch to main
	_, mergeErr := services.MergeBranchFastForward(ctx, repo.LocalPath, specTask.BranchName, repo.DefaultBranch)
	if mergeErr != nil {
		// Merge failed (not a fast-forward) - tell agent to rebase/merge main
		log.Warn().
			Err(mergeErr).
			Str("task_id", specTask.ID).
			Str("source_branch", specTask.BranchName).
			Str("target_branch", repo.DefaultBranch).
			Msg("Fast-forward merge failed - asking agent to rebase")

		// Don't record approval yet - user needs to review after rebase
		// Keep in implementation_review status so agent stays alive
		specTask.Status = types.TaskStatusImplementationReview
		specTask.StatusUpdatedAt = &now
		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
			return
		}

		// Send rebase instruction to agent
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			message, err := prompts.RebaseRequiredInstruction(specTask.BranchName, repo.DefaultBranch)
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", specTask.ID).
					Msg("Failed to generate rebase instruction")
				return
			}

			_, err = s.sendMessageToSpecTaskAgent(context.Background(), specTask, message, "")
			if err != nil {
				log.Error().
					Err(err).
					Str("task_id", specTask.ID).
					Msg("Failed to send rebase instruction to agent")
			} else {
				log.Info().
					Str("task_id", specTask.ID).
					Str("branch_name", specTask.BranchName).
					Str("default_branch", repo.DefaultBranch).
					Msg("Sent rebase instruction to agent")
			}
		}()

		// Return task with merge conflict flag set (already saved to DB)
		writeResponse(w, specTask, http.StatusOK)
		return
	}

	// For external repos, push the merged default branch to upstream
	if repo.IsExternal && repo.ExternalURL != "" {
		if pushErr := s.gitRepositoryService.PushBranchToRemote(ctx, repo.ID, repo.DefaultBranch, false); pushErr != nil {
			// Push failed - rollback merge and return error
			log.Error().
				Err(pushErr).
				Str("task_id", specTask.ID).
				Str("branch", repo.DefaultBranch).
				Msg("Failed to push merged branch to upstream - rolling back")

			if oldDefaultBranchRef != "" {
				if rollbackErr := services.UpdateBranchRef(ctx, repo.LocalPath, repo.DefaultBranch, oldDefaultBranchRef); rollbackErr != nil {
					log.Error().
						Err(rollbackErr).
						Str("task_id", specTask.ID).
						Str("branch", repo.DefaultBranch).
						Msg("Failed to rollback branch after push failure")
				}
			}

			http.Error(w, fmt.Sprintf("Failed to push merge to upstream: %s", pushErr.Error()), http.StatusInternalServerError)
			return
		}

		log.Info().
			Str("task_id", specTask.ID).
			Str("branch", repo.DefaultBranch).
			Msg("Pushed merged branch to upstream")
	}

	// Merge succeeded - now record the approval
	specTask.ImplementationApprovedBy = user.ID
	specTask.ImplementationApprovedAt = &now
	specTask.MergedToMain = true
	specTask.MergedAt = &now
	specTask.Status = types.TaskStatusDone
	specTask.StatusUpdatedAt = &now
	specTask.CompletedAt = &now

	log.Info().
		Str("task_id", specTask.ID).
		Str("source_branch", specTask.BranchName).
		Str("target_branch", repo.DefaultBranch).
		Msg("Server-side merge completed")

	// Updating spec task
	if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	log.Info().
		Str("task_id", specTask.ID).
		Str("branch_name", specTask.BranchName).
		Msg("Implementation approved - branch merged server-side, task complete")

	// Trigger golden Docker cache build if enabled for this project
	if s.goldenBuildService != nil {
		s.goldenBuildService.TriggerGoldenBuild(ctx, project)
	}

	// Return updated task
	writeResponse(w, specTask, http.StatusOK)
}

// branchHasCommitsAhead checks if a feature branch has commits ahead of the default branch
func (s *HelixAPIServer) branchHasCommitsAhead(ctx context.Context, repoPath, featureBranch, defaultBranch string) (bool, error) {
	ahead, _, err := services.GetDivergence(ctx, repoPath, featureBranch, defaultBranch)
	if err != nil {
		return false, err
	}
	return ahead > 0, nil
}

// getPullRequestContentForTask reads pull_request.md from helix-specs branch for a task.
// Returns (title, description, found). If not found or error, returns empty strings and false.
func (s *HelixAPIServer) getPullRequestContentForTask(repoPath string, task *types.SpecTask) (string, string, bool) {
	if task.DesignDocPath == "" {
		return "", "", false
	}

	// Read from helix-specs branch
	filePath := "design/tasks/" + task.DesignDocPath + "/pull_request.md"
	cmd := exec.Command("git", "show", "helix-specs:"+filePath)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// File doesn't exist or other error - this is expected for tasks without pull_request.md
		return "", "", false
	}

	return parsePullRequestMarkdownForTask(string(output))
}

// parsePullRequestMarkdownForTask parses a pull_request.md file content into title and description.
// Format: First line (with optional "# " prefix) = title, everything after first blank line = description.
func parsePullRequestMarkdownForTask(content string) (string, string, bool) {
	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return "", "", false
	}

	// First line is title (strip # prefix if present)
	title := strings.TrimSpace(lines[0])
	title = strings.TrimPrefix(title, "# ")
	title = strings.TrimSpace(title)

	if title == "" {
		return "", "", false
	}

	// Find first blank line, everything after is description
	var descLines []string
	foundBlank := false
	for i := 1; i < len(lines); i++ {
		if !foundBlank && strings.TrimSpace(lines[i]) == "" {
			foundBlank = true
			continue
		}
		if foundBlank {
			descLines = append(descLines, lines[i])
		}
	}

	description := strings.TrimSpace(strings.Join(descLines, "\n"))
	return title, description, true
}

// getSpecDocsBaseURLForTask builds a URL to view spec docs in the external repo's web UI.
func getSpecDocsBaseURLForTask(repo *types.GitRepository, designDocPath string) string {
	if repo.ExternalURL == "" {
		return ""
	}

	baseURL := strings.TrimSuffix(repo.ExternalURL, ".git")

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeGitHub:
		return fmt.Sprintf("%s/blob/helix-specs/design/tasks/%s", baseURL, designDocPath)
	case types.ExternalRepositoryTypeGitLab:
		return fmt.Sprintf("%s/-/blob/helix-specs/design/tasks/%s", baseURL, designDocPath)
	case types.ExternalRepositoryTypeADO:
		return fmt.Sprintf("%s?path=/design/tasks/%s&version=GBhelix-specs", baseURL, designDocPath)
	case types.ExternalRepositoryTypeBitbucket:
		return fmt.Sprintf("%s/src/helix-specs/design/tasks/%s", baseURL, designDocPath)
	default:
		return ""
	}
}

// buildPRFooterForTask generates the PR description footer.
func (s *HelixAPIServer) buildPRFooterForTask(ctx context.Context, repo *types.GitRepository, task *types.SpecTask) string {
	var parts []string

	// "Open in Helix" link
	helixBaseURL := s.Cfg.WebServer.URL
	orgName := ""
	if task.OrganizationID != "" {
		if org, err := s.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: task.OrganizationID}); err == nil && org != nil {
			orgName = org.Name
		}
	}
	if helixBaseURL != "" && orgName != "" && task.ProjectID != "" && task.ID != "" {
		helixTaskURL := fmt.Sprintf("%s/orgs/%s/projects/%s/tasks/%s",
			strings.TrimSuffix(helixBaseURL, "/"), orgName, task.ProjectID, task.ID)
		parts = append(parts, fmt.Sprintf("🔗 [Open in Helix](%s)", helixTaskURL))
	}

	// Spec doc links
	if task.DesignDocPath != "" {
		if specDocsURL := getSpecDocsBaseURLForTask(repo, task.DesignDocPath); specDocsURL != "" {
			parts = append(parts, fmt.Sprintf("📋 [Requirements](%s/requirements.md) | [Design](%s/design.md) | [Tasks](%s/tasks.md)",
				specDocsURL, specDocsURL, specDocsURL))
		}
	}

	// Helix branding
	parts = append(parts, "🚀 Built with [Helix](https://helix.ml)")

	return "---\n" + strings.Join(parts, " | ")
}

// ensurePullRequestForTask creates a PR for a spec task if one doesn't exist
func (s *HelixAPIServer) ensurePullRequestForTask(ctx context.Context, repo *types.GitRepository, task *types.SpecTask) error {
	if repo.ExternalURL == "" {
		return nil
	}

	branch := task.BranchName
	log.Info().Str("repo_id", repo.ID).Str("branch", branch).Str("task_id", task.ID).Msg("Ensuring pull request for task")

	// Push branch to remote first
	if err := s.gitRepositoryService.WithRepoLock(repo.ID, func() error {
		return s.gitRepositoryService.PushBranchToRemote(ctx, repo.ID, branch, false)
	}); err != nil {
		return fmt.Errorf("failed to push branch: %w", err)
	}

	// Check if PR already exists
	prs, err := s.gitRepositoryService.ListPullRequests(ctx, repo.ID)
	if err != nil {
		return fmt.Errorf("failed to list PRs: %w", err)
	}

	sourceBranchRef := "refs/heads/" + branch
	for _, pr := range prs {
		if pr.SourceBranch == sourceBranchRef && pr.State == types.PullRequestStateOpen {
			if task.PullRequestID != pr.ID {
				task.PullRequestID = pr.ID
				task.UpdatedAt = time.Now()
				if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
					log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with existing PR ID")
				}
			}
			log.Info().Str("pr_id", pr.ID).Str("branch", branch).Msg("Pull request already exists")
			return nil
		}
	}

	// Try to get custom PR content from pull_request.md
	title, description, found := s.getPullRequestContentForTask(repo.LocalPath, task)
	if !found {
		// Fallback to existing behavior
		title = task.Name
		description = task.Description
		log.Debug().Str("task_id", task.ID).Msg("No pull_request.md found, using task name/description")
	} else {
		log.Info().Str("task_id", task.ID).Msg("Using pull_request.md for PR content")
	}

	// Append footer
	footer := s.buildPRFooterForTask(ctx, repo, task)
	description = description + "\n\n" + footer

	// Create new PR
	prID, err := s.gitRepositoryService.CreatePullRequest(ctx, repo.ID, title, description, branch, repo.DefaultBranch)
	if err != nil {
		return fmt.Errorf("failed to create PR: %w", err)
	}

	task.PullRequestID = prID
	task.UpdatedAt = time.Now()
	if err := s.Store.UpdateSpecTask(ctx, task); err != nil {
		log.Error().Err(err).Str("task_id", task.ID).Msg("Failed to update task with new PR ID")
	}
	log.Info().Str("pr_id", prID).Str("branch", branch).Str("task_id", task.ID).Msg("Created pull request for task")
	return nil
}

func (s *HelixAPIServer) shouldOpenPullRequest(repo *types.GitRepository) bool {
	switch {
	case repo.ExternalType == types.ExternalRepositoryTypeGitHub && repo.OAuthConnectionID != "":
		// Github OAuth connection ID set
		return true
	case repo.ExternalType == types.ExternalRepositoryTypeGitHub:
		if repo.Username != "" && repo.Password != "" {
			return true
		}

		if repo.GitHub != nil && repo.GitHub.PersonalAccessToken != "" {
			return true
		}

		// Github PRs implemented
		return true
	case repo.AzureDevOps != nil:
		// ADO PRs implemented
		return true
	}
	return false
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

	// Stop the container via Hydra executor if there's an active session
	if specTask.PlanningSessionID != "" && s.externalAgentExecutor != nil {
		log.Info().
			Str("task_id", specTask.ID).
			Str("session_id", specTask.PlanningSessionID).
			Str("user_id", user.ID).
			Msg("Stopping agent container via Hydra")

		if err := s.externalAgentExecutor.StopDesktop(ctx, specTask.PlanningSessionID); err != nil {
			log.Warn().
				Err(err).
				Str("session_id", specTask.PlanningSessionID).
				Msg("Failed to stop agent container (may already be stopped)")
			// Don't return error - container might already be gone
		} else {
			log.Info().
				Str("task_id", specTask.ID).
				Str("session_id", specTask.PlanningSessionID).
				Msg("Agent container stopped successfully")
		}
	} else {
		log.Info().
			Str("task_id", specTask.ID).
			Str("user_id", user.ID).
			Bool("has_session", specTask.PlanningSessionID != "").
			Bool("has_executor", s.externalAgentExecutor != nil).
			Msg("Agent stop requested (no container to stop)")
	}

	// Return task
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(specTask)
}
