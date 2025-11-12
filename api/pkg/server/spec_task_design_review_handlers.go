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
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Design Review Handlers - Simple versions

// listDesignReviews lists design reviews for a spec task
// @Summary List design reviews
// @Description List all design reviews for a spec task
// @Tags spec-tasks
// @Produce json
// @Param spec_task_id path string true "SpecTask ID"
// @Success 200 {object} types.SpecTaskDesignReviewListResponse
// @Failure 400 {object} types.APIError
// @Failure 403 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews [get]
// @Security BearerAuth
func (s *HelixAPIServer) listDesignReviews(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, fmt.Sprintf("Failed to get spec task: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get project: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	// For personal projects (no organization), check if user is the project owner
	if project.OrganizationID == "" {
		if user.ID != project.UserID {
			log.Warn().
				Str("user_id", user.ID).
				Str("project_owner", project.UserID).
				Str("project_id", project.ID).
				Msg("User is not the owner of this personal project")
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
		// User is the owner - authorized
	} else {
		// Organization project - use RBAC
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, types.ActionGet); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("org_id", project.OrganizationID).
				Msg("User not authorized to read spec task design reviews")
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	reviews, err := s.Store.ListSpecTaskDesignReviews(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Self-healing: If task is in spec_review but has no reviews, auto-create one from git
	if len(reviews) == 0 && specTask.Status == types.TaskStatusSpecReview {
		log.Info().
			Str("spec_task_id", specTaskID).
			Msg("No design reviews found for task in spec_review status - auto-creating from git")

		// Get project to find repository
		project, err := s.Store.GetProject(ctx, specTask.ProjectID)
		if err == nil && project.DefaultRepoID != "" {
			repo, err := s.Store.GetGitRepository(ctx, project.DefaultRepoID)
			if err == nil && repo != nil {
				// Create review from git asynchronously (don't block response)
				go s.backfillDesignReviewFromGit(context.Background(), specTaskID, repo.LocalPath)
			}
		}
	}

	response := &types.SpecTaskDesignReviewListResponse{
		Reviews: reviews,
		Total:   len(reviews),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getDesignReview returns a specific design review by ID with all details
// @Summary Get design review details
// @Description Get a specific design review for a spec task with comments and spec task details
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Success 200 {object} types.SpecTaskDesignReviewDetailResponse
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id} [get]
// @Security BearerAuth
func (s *HelixAPIServer) getDesignReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Allow if user is project owner, otherwise check access grants
	if user.ID != project.UserID {
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, "read"); err != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	review, err := s.Store.GetSpecTaskDesignReview(ctx, reviewID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	comments, err := s.Store.ListSpecTaskDesignReviewComments(ctx, reviewID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := &types.SpecTaskDesignReviewDetailResponse{
		Review:   *review,
		Comments: comments,
		SpecTask: *specTask,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// submitDesignReview approves or requests changes for a design review
// @Summary Submit design review decision
// @Description Approve or request changes for a design review
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Param request body types.SpecTaskDesignReviewSubmitRequest true "Review decision"
// @Success 200 {object} types.SpecTaskDesignReview
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/submit [post]
// @Security BearerAuth
func (s *HelixAPIServer) submitDesignReview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	var req types.SpecTaskDesignReviewSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Allow if user is project owner, otherwise check access grants
	if user.ID != project.UserID {
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, "update"); err != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	review, err := s.Store.GetSpecTaskDesignReview(ctx, reviewID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch req.Decision {
	case "approve":
		review.Status = types.SpecTaskDesignReviewStatusApproved
		now := time.Now()
		review.ApprovedAt = &now
		review.OverallComment = req.OverallComment

		// Get base branch for implementation
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

		// Generate feature branch name
		branchName := fmt.Sprintf("feature/%s-%s", specTask.Name, specTask.ID[:8])
		branchName = sanitizeBranchName(branchName)

		// Move to implementation status
		specTask.Status = types.TaskStatusImplementation
		specTask.BranchName = branchName
		specTask.SpecApprovedBy = user.ID
		specTask.SpecApprovedAt = &now
		specTask.StartedAt = &now

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Send implementation instruction to agent (reuse existing session)
		sessionID := specTask.PlanningSessionID
		if sessionID == "" {
			sessionID = specTask.SpecSessionID // Legacy fallback
		}

		if sessionID != "" {
			agentInstructionService := services.NewAgentInstructionService(s.Store)
			go func() {
				err := agentInstructionService.SendApprovalInstruction(
					context.Background(),
					sessionID,
					branchName,
					baseBranch,
				)
				if err != nil {
					log.Error().
						Err(err).
						Str("task_id", specTask.ID).
						Str("session_id", sessionID).
						Msg("Failed to send approval instruction to agent")
				}
			}()

			log.Info().
				Str("task_id", specTask.ID).
				Str("session_id", sessionID).
				Str("branch_name", branchName).
				Msg("Design approved - sent implementation instruction to existing agent")
		} else {
			log.Warn().
				Str("task_id", specTask.ID).
				Msg("No session ID found - agent will not receive implementation instruction")
		}

	case "request_changes":
		review.Status = types.SpecTaskDesignReviewStatusChangesRequested
		now := time.Now()
		review.RejectedAt = &now
		review.OverallComment = req.OverallComment

		specTask.Status = types.TaskStatusSpecRevision
		specTask.SpecRevisionCount++

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// TODO: Notify agent of requested changes via WebSocket
		// For now, just log the event - agent will see it when they next interact
		log.Info().
			Str("spec_task_id", specTask.ID).
			Str("review_id", review.ID).
			Int("revision_count", specTask.SpecRevisionCount).
			Msg("[DesignReview] Changes requested, agent should be notified")
	default:
		http.Error(w, "Invalid decision", http.StatusBadRequest)
		return
	}

	if err := s.Store.UpdateSpecTaskDesignReview(ctx, review); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// createDesignReviewComment creates a new inline or general comment on a design review
// @Summary Create design review comment
// @Description Create a new comment on a design review document
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Param request body types.SpecTaskDesignReviewCommentCreateRequest true "Comment data"
// @Success 200 {object} types.SpecTaskDesignReviewComment
// @Failure 400 {object} system.HTTPError
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments [post]
// @Security BearerAuth
func (s *HelixAPIServer) createDesignReviewComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	var req types.SpecTaskDesignReviewCommentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Allow if user is project owner, otherwise check access grants
	if user.ID != project.UserID {
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, "update"); err != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	comment := &types.SpecTaskDesignReviewComment{
		ReviewID:     reviewID,
		CommentedBy:  user.ID,
		DocumentType: req.DocumentType,
		SectionPath:  req.SectionPath,
		LineNumber:   req.LineNumber,
		QuotedText:   req.QuotedText,
		StartOffset:  req.StartOffset,
		EndOffset:    req.EndOffset,
		CommentText:  req.CommentText,
		CommentType:  req.CommentType,
		Resolved:     false,
	}

	if err := s.Store.CreateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send comment to agent session asynchronously
	go func() {
		if err := s.sendCommentToAgent(context.Background(), specTask, comment); err != nil {
			log.Error().
				Err(err).
				Str("comment_id", comment.ID).
				Str("spec_task_id", specTask.ID).
				Msg("Failed to send comment to agent (async)")
		}
	}()

	review, err := s.Store.GetSpecTaskDesignReview(ctx, reviewID)
	if err == nil && review.Status == types.SpecTaskDesignReviewStatusPending {
		review.Status = types.SpecTaskDesignReviewStatusInReview
		if review.ReviewerID == "" {
			review.ReviewerID = user.ID
		}
		s.Store.UpdateSpecTaskDesignReview(ctx, review)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

// listDesignReviewComments lists all comments for a design review
// @Summary List design review comments
// @Description Get all comments for a specific design review
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Success 200 {object} types.SpecTaskDesignReviewCommentListResponse
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments [get]
// @Security BearerAuth
func (s *HelixAPIServer) listDesignReviewComments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Allow if user is project owner, otherwise check access grants
	if user.ID != project.UserID {
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, "read"); err != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	comments, err := s.Store.ListSpecTaskDesignReviewComments(ctx, reviewID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := &types.SpecTaskDesignReviewCommentListResponse{
		Comments: comments,
		Total:    len(comments),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// resolveDesignReviewComment marks a comment as resolved
// @Summary Resolve design review comment
// @Description Mark a design review comment as resolved
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Param comment_id path string true "Comment ID"
// @Success 200 {object} types.SpecTaskDesignReviewComment
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comments/{comment_id}/resolve [post]
// @Security BearerAuth
func (s *HelixAPIServer) resolveDesignReviewComment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	commentID := vars["comment_id"]

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Get project to check ownership
	project, err := s.Store.GetProject(ctx, specTask.ProjectID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Allow if user is project owner, otherwise check access grants
	if user.ID != project.UserID {
		if err := s.authorizeUserToResource(ctx, user, project.OrganizationID, specTask.ProjectID, types.ResourceProject, "update"); err != nil {
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	comment, err := s.Store.GetSpecTaskDesignReviewComment(ctx, commentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	comment.Resolved = true
	comment.ResolvedBy = user.ID
	comment.ResolutionReason = "manual"
	now := time.Now()
	comment.ResolvedAt = &now

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}

// sendCommentToAgent sends a design review comment to the agent's session
func (s *HelixAPIServer) sendCommentToAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	comment *types.SpecTaskDesignReviewComment,
) error {
	if specTask.SpecSessionID == "" {
		log.Debug().
			Str("spec_task_id", specTask.ID).
			Msg("No spec session ID, skipping agent notification for comment")
		return nil
	}

	// Build prompt for agent
	documentTypeLabels := map[string]string{
		"requirements":        "Requirements Specification",
		"technical_design":    "Technical Design",
		"implementation_plan": "Implementation Plan",
	}
	docLabel := documentTypeLabels[comment.DocumentType]
	if docLabel == "" {
		docLabel = comment.DocumentType
	}

	promptText := fmt.Sprintf(`A reviewer left a comment on your design document:

**Document:** %s
**Quoted Text:**
> %s

**Comment:**
%s

Please respond to this comment and explain your approach. If the reviewer's feedback requires changes to the design, update the relevant document in your helix-specs repository and push your changes.

Your response will be shown to the reviewer in the design review interface.`,
		docLabel,
		comment.QuotedText,
		comment.CommentText)

	// Create interaction in agent's session
	interaction := &types.Interaction{
		ID:        system.GenerateInteractionID(),
		Created:   time.Now(),
		Updated:   time.Now(),
		SessionID: specTask.SpecSessionID,
		UserID:    comment.CommentedBy,
		// This is a text-only interaction (no files/images)
		PromptMessage: promptText,
		// Mode is inference (agent responds to prompt)
		Mode:  types.SessionModeInference,
		State: types.InteractionStateWaiting,
	}

	_, err := s.Store.CreateInteraction(ctx, interaction)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTask.ID).
			Str("comment_id", comment.ID).
			Msg("Failed to create interaction for design review comment")
		return fmt.Errorf("failed to create interaction for comment: %w", err)
	}

	// Store interaction ID in comment for linking responses
	comment.InteractionID = interaction.ID
	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		log.Error().
			Err(err).
			Str("comment_id", comment.ID).
			Str("interaction_id", interaction.ID).
			Msg("Failed to link interaction ID to comment")
		// Don't return error - interaction was created successfully
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("comment_id", comment.ID).
		Str("interaction_id", interaction.ID).
		Str("session_id", specTask.SpecSessionID).
		Msg("Sent design review comment to agent session")

	return nil
}

// linkAgentResponseToComment links an agent's interaction response to the design review comment
func (s *HelixAPIServer) linkAgentResponseToComment(
	ctx context.Context,
	interaction *types.Interaction,
) error {
	if interaction.ID == "" {
		return fmt.Errorf("interaction ID is empty")
	}

	// Find comment by interaction ID
	comment, err := s.Store.GetCommentByInteractionID(ctx, interaction.ID)
	if err != nil {
		// Not all interactions are linked to comments - this is normal
		return fmt.Errorf("no comment found for interaction %s: %w", interaction.ID, err)
	}

	// Update comment with agent response
	comment.AgentResponse = interaction.ResponseMessage
	now := time.Now()
	comment.AgentResponseAt = &now

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		return fmt.Errorf("failed to update comment with agent response: %w", err)
	}

	log.Info().
		Str("comment_id", comment.ID).
		Str("interaction_id", interaction.ID).
		Int("response_length", len(interaction.ResponseMessage)).
		Msg("Linked agent response to design review comment")

	return nil
}

// backfillDesignReviewFromGit creates a design review from the current state of helix-specs branch
// Used for self-healing when a task is in spec_review but has no review record
func (s *HelixAPIServer) backfillDesignReviewFromGit(ctx context.Context, specTaskID, repoPath string) {
	log.Info().
		Str("spec_task_id", specTaskID).
		Msg("Backfilling design review from git")

	// Get current commit hash from helix-specs branch
	cmd := exec.Command("git", "rev-parse", "helix-specs")
	cmd.Dir = repoPath
	hashOutput, err := cmd.Output()
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to get commit hash from helix-specs branch")
		return
	}
	commitHash := strings.TrimSpace(string(hashOutput))

	// List all files in helix-specs branch to find task directory
	cmd = exec.Command("git", "ls-tree", "--name-only", "-r", "helix-specs")
	cmd.Dir = repoPath
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to list files in helix-specs branch")
		return
	}

	// Find task directory by searching for task ID in file paths
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var taskDir string
	for _, file := range files {
		if strings.Contains(file, specTaskID) {
			// Extract directory path
			parts := strings.Split(file, "/")
			if len(parts) >= 3 {
				taskDir = strings.Join(parts[:len(parts)-1], "/")
				break
			}
		}
	}

	if taskDir == "" {
		log.Warn().
			Str("spec_task_id", specTaskID).
			Msg("No task directory found in helix-specs branch for backfill")
		return
	}

	// Read design documents from task directory
	docs := make(map[string]string)
	docFilenames := []string{"requirements.md", "design.md", "tasks.md"}

	for _, filename := range docFilenames {
		filePath := fmt.Sprintf("%s/%s", taskDir, filename)
		cmd := exec.Command("git", "show", fmt.Sprintf("helix-specs:%s", filePath))
		cmd.Dir = repoPath
		output, err := cmd.Output()
		if err != nil {
			log.Debug().
				Err(err).
				Str("filename", filename).
				Msg("Design doc file not found during backfill")
			continue
		}
		docs[filename] = string(output)
	}

	// Create design review record
	review := &types.SpecTaskDesignReview{
		ID:                 system.GenerateUUID(),
		SpecTaskID:         specTaskID,
		Status:             types.SpecTaskDesignReviewStatusPending,
		RequirementsSpec:   docs["requirements.md"],
		TechnicalDesign:    docs["design.md"],
		ImplementationPlan: docs["tasks.md"],
		GitBranch:          "helix-specs",
		GitCommitHash:      commitHash,
		GitPushedAt:        time.Now(),
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := s.Store.CreateSpecTaskDesignReview(ctx, review); err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTaskID).
			Msg("Failed to backfill design review")
		return
	}

	log.Info().
		Str("review_id", review.ID).
		Str("spec_task_id", specTaskID).
		Msg("✅ Design review backfilled successfully from git")
}
