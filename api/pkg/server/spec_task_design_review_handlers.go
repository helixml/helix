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
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// Design Review Handlers - Simple versions

// ResumeCommentQueueProcessing resumes processing of any queued comments after server restart.
// This should be called during server startup.
// It:
// 1. Resets any comments stuck in "processing" state (RequestID set but no response)
// 2. Triggers processing for all sessions that have pending comments
func (s *HelixAPIServer) ResumeCommentQueueProcessing(ctx context.Context) {
	log.Info().Msg("🔄 Resuming comment queue processing after startup...")

	// Step 1: Reset any comments that were mid-processing when server crashed
	resetCount, err := s.Store.ResetStuckComments(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reset stuck comments")
	} else if resetCount > 0 {
		log.Info().Int64("count", resetCount).Msg("✅ Reset stuck comments (were mid-processing during crash)")
	}

	// Step 2: Find all sessions with pending comments and trigger processing
	sessionIDs, err := s.Store.GetSessionsWithPendingComments(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get sessions with pending comments")
		return
	}

	if len(sessionIDs) == 0 {
		log.Info().Msg("✅ No pending comments to resume")
		return
	}

	log.Info().Int("session_count", len(sessionIDs)).Msg("📋 Found sessions with pending comments, triggering processing...")

	// Trigger processing for each session
	for _, sessionID := range sessionIDs {
		go s.processNextCommentInQueue(ctx, sessionID)
	}

	log.Info().Int("session_count", len(sessionIDs)).Msg("✅ Comment queue processing resumed")
}

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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionGet); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionGet); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to read design review")
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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionUpdate); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to submit design review")
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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionUpdate); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to create design review comment")
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

	log.Info().
		Str("comment_id", comment.ID).
		Str("spec_task_id", specTask.ID).
		Str("planning_session_id", specTask.PlanningSessionID).
		Msg("📝 Comment created, sending to agent...")

	// Send comment to agent session synchronously so we can return the request_id
	// The frontend can then subscribe to the stream endpoint for real-time response
	if err := s.sendCommentToAgent(ctx, specTask, comment); err != nil {
		log.Error().
			Err(err).
			Str("comment_id", comment.ID).
			Str("spec_task_id", specTask.ID).
			Str("planning_session_id", specTask.PlanningSessionID).
			Msg("❌ Failed to send comment to agent (will retry via polling)")
		// Don't fail the request - comment is still created, agent response will be linked via polling
	} else {
		log.Info().
			Str("comment_id", comment.ID).
			Str("spec_task_id", specTask.ID).
			Msg("✅ Comment queued for agent successfully")
	}

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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionGet); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to list design review comments")
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

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionUpdate); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to resolve design review comment")
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

// getDesignReviewCommentQueueStatus returns the current comment being processed and queue status
// @Summary Get comment queue status
// @Description Get the current comment being processed and the queue of pending comments for a review
// @Tags SpecTasks
// @Accept json
// @Produce json
// @Param spec_task_id path string true "Spec Task ID"
// @Param review_id path string true "Design Review ID"
// @Success 200 {object} types.CommentQueueStatusResponse
// @Failure 403 {object} system.HTTPError
// @Failure 500 {object} system.HTTPError
// @Router /api/v1/spec-tasks/{spec_task_id}/design-reviews/{review_id}/comment-queue-status [get]
// @Security BearerAuth
func (s *HelixAPIServer) getDesignReviewCommentQueueStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]

	specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Task creator always has access
	if user.ID != specTask.CreatedBy {
		// Otherwise check project authorization
		if err := s.authorizeUserToProjectByID(ctx, user, specTask.ProjectID, types.ActionGet); err != nil {
			log.Warn().
				Err(err).
				Str("user_id", user.ID).
				Str("project_id", specTask.ProjectID).
				Str("spec_task_creator", specTask.CreatedBy).
				Msg("User not authorized to get comment queue status")
			http.Error(w, "Not authorized", http.StatusForbidden)
			return
		}
	}

	sessionID := specTask.PlanningSessionID
	currentCommentID := s.GetCurrentCommentForSession(sessionID)
	queuedCommentIDs := s.GetCommentQueueForSession(sessionID)

	response := &types.CommentQueueStatusResponse{
		CurrentCommentID:  currentCommentID,
		QueuedCommentIDs:  queuedCommentIDs,
		PlanningSessionID: sessionID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// queueCommentForAgent adds a comment to the database queue for the agent to respond to.
// Comments are processed one at a time per planning session to avoid interleaving responses.
// DATABASE-PRIMARY: Uses QueuedAt field for queue state (restart-resilient).
func (s *HelixAPIServer) queueCommentForAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	comment *types.SpecTaskDesignReviewComment,
) error {
	if specTask.PlanningSessionID == "" {
		log.Debug().
			Str("spec_task_id", specTask.ID).
			Msg("No planning session ID, skipping agent notification for comment")
		return nil
	}

	sessionID := specTask.PlanningSessionID

	// Set QueuedAt to mark comment as queued for processing
	now := time.Now()
	comment.QueuedAt = &now

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		return fmt.Errorf("failed to queue comment for agent: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("comment_id", comment.ID).
		Time("queued_at", now).
		Msg("Queued comment for agent response (database-backed)")

	// Trigger processing - this will check if there's already a comment being processed
	// Use context.Background() because this runs async after HTTP request completes
	go s.processNextCommentInQueue(context.Background(), sessionID)

	return nil
}

// processNextCommentInQueue processes the next comment in the database queue for a session.
// DATABASE-PRIMARY: Uses database to check queue state (restart-resilient).
// The RequestID field serves as the "being processed" marker in the database.
func (s *HelixAPIServer) processNextCommentInQueue(ctx context.Context, sessionID string) {
	// Check if there's already a comment being processed (database check)
	isProcessing, err := s.Store.IsCommentBeingProcessedForSession(ctx, sessionID)
	if err != nil {
		log.Error().Err(err).Str("session_id", sessionID).Msg("Failed to check if comment is being processed")
		return
	}
	if isProcessing {
		log.Debug().
			Str("session_id", sessionID).
			Msg("Comment already being processed (database check), skipping")
		return
	}

	// Get next queued comment from database
	comment, err := s.Store.GetNextQueuedCommentForSession(ctx, sessionID)
	if err != nil {
		// No comments in queue (gorm.ErrRecordNotFound) - this is normal
		log.Debug().Str("session_id", sessionID).Msg("No queued comments to process")
		return
	}

	log.Info().
		Str("session_id", sessionID).
		Str("comment_id", comment.ID).
		Msg("Processing next comment from database queue")

	// Fetch spec task for the comment
	review, err := s.Store.GetSpecTaskDesignReview(ctx, comment.ReviewID)
	if err != nil {
		log.Error().
			Err(err).
			Str("review_id", comment.ReviewID).
			Msg("Failed to fetch review for comment processing")
		// Clear QueuedAt and try next
		comment.QueuedAt = nil
		if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); updateErr != nil {
			log.Error().Err(updateErr).Str("comment_id", comment.ID).Msg("Failed to clear QueuedAt")
		}
		go s.processNextCommentInQueue(ctx, sessionID)
		return
	}

	specTask, err := s.Store.GetSpecTask(ctx, review.SpecTaskID)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", review.SpecTaskID).
			Msg("Failed to fetch spec task for comment processing")
		// Clear QueuedAt and try next
		comment.QueuedAt = nil
		if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); updateErr != nil {
			log.Error().Err(updateErr).Str("comment_id", comment.ID).Msg("Failed to clear QueuedAt")
		}
		go s.processNextCommentInQueue(ctx, sessionID)
		return
	}

	// Now actually send the comment to the agent
	// sendCommentToAgentNow sets RequestID on the comment, marking it as "being processed"
	err = s.sendCommentToAgentNow(ctx, specTask, comment)
	if err != nil {
		log.Error().
			Err(err).
			Str("comment_id", comment.ID).
			Msg("Failed to send comment to agent")
		// Clear QueuedAt and try next
		comment.QueuedAt = nil
		if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); updateErr != nil {
			log.Error().Err(updateErr).Str("comment_id", comment.ID).Msg("Failed to clear QueuedAt")
		}
		go s.processNextCommentInQueue(ctx, sessionID)
		return
	}

	// Comment sent successfully - start timeout to handle agent not responding
	// 2 minute timeout for agent to respond to the comment
	const commentResponseTimeout = 2 * time.Minute
	s.sessionCommentMutex.Lock()
	// Cancel any existing timeout for this session
	if existingTimer := s.sessionCommentTimeout[sessionID]; existingTimer != nil {
		existingTimer.Stop()
	}
	commentID := comment.ID
	s.sessionCommentTimeout[sessionID] = time.AfterFunc(commentResponseTimeout, func() {
		log.Warn().
			Str("session_id", sessionID).
			Str("comment_id", commentID).
			Msg("Comment response timeout - agent did not respond within 2 minutes")

		// Re-fetch comment from database to check current state
		currentComment, fetchErr := s.Store.GetSpecTaskDesignReviewComment(ctx, commentID)
		if fetchErr != nil {
			log.Error().Err(fetchErr).Str("comment_id", commentID).Msg("Failed to fetch comment for timeout check")
			return
		}

		// Only mark as timed out if still processing (RequestID set and no response)
		if currentComment.RequestID != "" && currentComment.AgentResponse == "" {
			currentComment.AgentResponse = "[Agent did not respond - try sending your comment again]"
			currentComment.RequestID = ""
			currentComment.QueuedAt = nil
			if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, currentComment); updateErr != nil {
				log.Error().Err(updateErr).Str("comment_id", commentID).Msg("Failed to update timed-out comment")
			}

			// Process next comment in queue
			go s.processNextCommentInQueue(ctx, sessionID)
		}
	})
	s.sessionCommentMutex.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Str("comment_id", comment.ID).
		Msg("Comment sent to agent, started 2-minute response timeout")
}

// findConnectedSessionForSpecTask finds an active WebSocket connection for a spec task
// It first tries the planning session ID, then searches for any connected session with matching spec task ID
func (s *HelixAPIServer) findConnectedSessionForSpecTask(ctx context.Context, specTask *types.SpecTask) (string, error) {
	// First, try the planning session ID directly
	if specTask.PlanningSessionID != "" {
		if _, exists := s.externalAgentWSManager.getConnection(specTask.PlanningSessionID); exists {
			log.Debug().
				Str("spec_task_id", specTask.ID).
				Str("session_id", specTask.PlanningSessionID).
				Msg("Found WebSocket connection for planning session ID")
			return specTask.PlanningSessionID, nil
		}
	}

	// PlanningSessionID not connected - search for any connected session with this spec task ID
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("planning_session_id", specTask.PlanningSessionID).
		Msg("PlanningSessionID not connected, searching for alternate connected session")

	// Get all connected session IDs
	connectedSessions := s.externalAgentWSManager.listConnections()

	for _, conn := range connectedSessions {
		// Look up the session to check its SpecTaskID
		session, err := s.Store.GetSession(ctx, conn.SessionID)
		if err != nil {
			continue // Session not found or error, skip
		}

		// Check if this session is for our spec task
		if session.Metadata.SpecTaskID == specTask.ID {
			log.Info().
				Str("spec_task_id", specTask.ID).
				Str("found_session_id", conn.SessionID).
				Str("original_planning_session_id", specTask.PlanningSessionID).
				Msg("✅ Found alternate connected session for spec task")
			return conn.SessionID, nil
		}
	}

	return "", fmt.Errorf("no WebSocket connection found for spec task %s (tried planning session %s and %d other connected sessions)",
		specTask.ID, specTask.PlanningSessionID, len(connectedSessions))
}

// sendCommentToAgentNow actually sends a comment to the agent (called from queue processor)
func (s *HelixAPIServer) sendCommentToAgentNow(
	ctx context.Context,
	specTask *types.SpecTask,
	comment *types.SpecTaskDesignReviewComment,
) error {
	// Build prompt for agent using the shared helper
	promptText := services.BuildCommentPrompt(specTask, comment)

	// Send via the unified helper, notifying the commenter of responses
	requestID, err := s.sendMessageToSpecTaskAgent(ctx, specTask, promptText, comment.CommentedBy)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTask.ID).
			Str("comment_id", comment.ID).
			Msg("Failed to send comment to agent via websocket")
		return err
	}

	// Store the requestID on the comment in the database for persistent linking
	comment.RequestID = requestID
	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		log.Error().
			Err(err).
			Str("comment_id", comment.ID).
			Msg("Failed to update comment with request_id")
		// Continue anyway - the comment was sent, just won't have response linking
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("comment_id", comment.ID).
		Str("request_id", requestID).
		Msg("Sent design review comment to agent via websocket (with response mapping)")

	return nil
}

// sendCommentToAgent queues a comment for agent response (backwards compatible wrapper)
func (s *HelixAPIServer) sendCommentToAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	comment *types.SpecTaskDesignReviewComment,
) error {
	return s.queueCommentForAgent(ctx, specTask, comment)
}

// GetCurrentCommentForSession returns the comment ID currently being processed for a session.
// DATABASE-PRIMARY: Queries database for comment with request_id set (being processed).
func (s *HelixAPIServer) GetCurrentCommentForSession(sessionID string) string {
	// A comment is "current" if it has request_id set (sent to agent, awaiting response)
	isProcessing, err := s.Store.IsCommentBeingProcessedForSession(context.Background(), sessionID)
	if err != nil || !isProcessing {
		return ""
	}

	// Find the comment that's being processed (has request_id set)
	comment, err := s.Store.GetPendingCommentByPlanningSessionID(context.Background(), sessionID)
	if err != nil {
		return ""
	}
	return comment.ID
}

// GetCommentQueueForSession returns the list of comment IDs waiting in queue for a session.
// DATABASE-PRIMARY: Queries database for comments with queued_at set but no request_id.
func (s *HelixAPIServer) GetCommentQueueForSession(sessionID string) []string {
	// Query all queued comments (queued_at set, request_id empty, no response yet)
	// For now, we use a simple approach - get the next one and return it
	// A proper implementation would add a ListQueuedCommentsForSession method
	comment, err := s.Store.GetNextQueuedCommentForSession(context.Background(), sessionID)
	if err != nil {
		return []string{}
	}
	// For now just return the next one - in a full implementation we'd list all queued
	return []string{comment.ID}
}

// linkAgentResponseToComment links an agent's interaction response to the design review comment
// This is called from handleMessageAdded when we have an interaction but no request_id
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

// updateCommentWithStreamingResponse updates a comment with streaming agent response content
// This is called during streaming (message_added events) - does NOT clear request_id or trigger next queue item
func (s *HelixAPIServer) updateCommentWithStreamingResponse(
	ctx context.Context,
	requestID string,
	responseContent string,
) error {
	if requestID == "" {
		return fmt.Errorf("request ID is empty")
	}

	// Look up comment by request ID from database
	comment, err := s.Store.GetCommentByRequestID(ctx, requestID)
	if err != nil {
		// Not all requests are linked to comments - this is normal
		return fmt.Errorf("no comment found for request %s: %w", requestID, err)
	}

	// Update comment with agent response (streaming update)
	comment.AgentResponse = responseContent
	now := time.Now()
	comment.AgentResponseAt = &now
	// NOTE: Do NOT clear request_id here - streaming is still in progress

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		return fmt.Errorf("failed to update comment with streaming response: %w", err)
	}

	log.Debug().
		Str("comment_id", comment.ID).
		Str("request_id", requestID).
		Int("response_length", len(responseContent)).
		Msg("Updated comment with streaming agent response")

	return nil
}

// finalizeCommentResponse marks a comment response as complete, clears request_id/queued_at and triggers next queue item
// This is called when message_completed event is received.
// DATABASE-PRIMARY: Uses database state only (no in-memory fallbacks).
func (s *HelixAPIServer) finalizeCommentResponse(
	ctx context.Context,
	requestID string,
) error {
	if requestID == "" {
		return fmt.Errorf("request ID is empty")
	}

	// Look up comment by request ID from database
	comment, err := s.Store.GetCommentByRequestID(ctx, requestID)
	if err != nil {
		// Not all requests are linked to comments - this is normal
		return fmt.Errorf("no comment found for request %s: %w", requestID, err)
	}

	// Clear both request_id and queued_at to mark as fully processed
	comment.RequestID = ""
	comment.QueuedAt = nil

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		return fmt.Errorf("failed to finalize comment response: %w", err)
	}

	log.Info().
		Str("comment_id", comment.ID).
		Str("original_request_id", requestID).
		Int("final_response_length", len(comment.AgentResponse)).
		Msg("✅ Finalized comment response (cleared request_id and queued_at)")

	// Get sessionID from database (primary mechanism)
	review, err := s.Store.GetSpecTaskDesignReview(ctx, comment.ReviewID)
	if err != nil {
		log.Error().
			Err(err).
			Str("comment_id", comment.ID).
			Str("review_id", comment.ReviewID).
			Msg("❌ Failed to get design review - cannot process next comment")
		return nil
	}

	specTask, err := s.Store.GetSpecTask(ctx, review.SpecTaskID)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", review.SpecTaskID).
			Msg("❌ Failed to get spec task - cannot process next comment")
		return nil
	}

	if specTask.PlanningSessionID == "" {
		log.Warn().
			Str("spec_task_id", specTask.ID).
			Msg("⚠️ SpecTask has empty PlanningSessionID - cannot process next comment")
		return nil
	}

	sessionID := specTask.PlanningSessionID

	// Cancel the timeout timer since we got a response
	s.sessionCommentMutex.Lock()
	if timer := s.sessionCommentTimeout[sessionID]; timer != nil {
		timer.Stop()
		delete(s.sessionCommentTimeout, sessionID)
	}
	s.sessionCommentMutex.Unlock()

	log.Info().
		Str("session_id", sessionID).
		Str("completed_comment", comment.ID).
		Msg("Comment response complete, checking for next in queue")

	// Call synchronously - we're already in a goroutine from handleMessageCompleted
	// No need for another async hop which adds latency
	s.processNextCommentInQueue(ctx, sessionID)

	return nil
}

// linkAgentResponseToCommentByRequestID is a legacy function - kept for backwards compatibility
// Use updateCommentWithStreamingResponse for streaming updates and finalizeCommentResponse for completion
func (s *HelixAPIServer) linkAgentResponseToCommentByRequestID(
	ctx context.Context,
	requestID string,
	responseContent string,
) error {
	// For backwards compatibility, this now just updates the streaming response
	return s.updateCommentWithStreamingResponse(ctx, requestID, responseContent)
}

// backfillDesignReviewFromGit creates a design review from the current state of helix-specs branch
// Used for self-healing when a task is in spec_review but has no review record
func (s *HelixAPIServer) backfillDesignReviewFromGit(ctx context.Context, specTaskID, repoPath string) {
	log.Info().
		Str("spec_task_id", specTaskID).
		Msg("Backfilling design review from git")

	// Get task to find DesignDocPath for directory lookup
	task, err := s.Store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		log.Error().Err(err).Str("spec_task_id", specTaskID).Msg("Failed to get task for backfill")
		return
	}

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

	// Find task directory - first try DesignDocPath, then fall back to specTaskID
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var taskDir string

	// First try DesignDocPath (new human-readable format)
	if task.DesignDocPath != "" {
		for _, file := range files {
			if strings.Contains(file, task.DesignDocPath) {
				parts := strings.Split(file, "/")
				if len(parts) >= 3 {
					taskDir = strings.Join(parts[:len(parts)-1], "/")
					break
				}
			}
		}
	}

	// Fall back to specTaskID for backwards compatibility
	if taskDir == "" {
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

// sanitizeBranchName sanitizes a string to be used as a git branch name
func sanitizeBranchName(name string) string {
	// Replace spaces with hyphens
	name = strings.ReplaceAll(name, " ", "-")
	// Remove special characters except hyphens and underscores
	result := strings.Builder{}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '/' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// sendMessageToSpecTaskAgent is the unified helper for sending messages to spec task agents via WebSocket
// It handles: finding connected session, generating request ID, setting up response routing, and sending
// Returns the generated requestID for callers that need to track responses
func (s *HelixAPIServer) sendMessageToSpecTaskAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	message string,
	notifyUserID string, // Optional: user to notify of responses (e.g., commenter). Empty = no extra notification
) (string, error) {
	// Find a connected session for this spec task
	sessionID, err := s.findConnectedSessionForSpecTask(ctx, specTask)
	if err != nil {
		return "", fmt.Errorf("no connected session found: %w", err)
	}

	// Generate request ID for tracking
	requestID := "req_" + system.GenerateUUID()

	// Store the requestID -> sessionID mapping for response routing
	if s.requestToSessionMapping == nil {
		s.requestToSessionMapping = make(map[string]string)
	}
	s.requestToSessionMapping[requestID] = sessionID

	// If a notifyUserID is provided, store it for response notification
	if notifyUserID != "" {
		if s.requestToCommenterMapping == nil {
			s.requestToCommenterMapping = make(map[string]string)
		}
		s.requestToCommenterMapping[requestID] = notifyUserID
	}

	// Send the message via WebSocket
	err = s.sendChatMessageToExternalAgent(sessionID, message, requestID)
	if err != nil {
		// Clean up mappings on failure
		delete(s.requestToSessionMapping, requestID)
		if notifyUserID != "" {
			delete(s.requestToCommenterMapping, requestID)
		}
		return "", fmt.Errorf("failed to send message via WebSocket: %w", err)
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("session_id", sessionID).
		Str("request_id", requestID).
		Msg("✅ Sent message to spec task agent via WebSocket")

	return requestID, nil
}

// sendApprovalInstructionToAgent sends the detailed implementation instruction to the agent via WebSocket
// This is called when a design review is approved
func (s *HelixAPIServer) sendApprovalInstructionToAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	branchName string,
	baseBranch string,
	primaryRepoName string,
) error {
	// Fetch guidelines from project and organization
	guidelines := s.getGuidelinesForSpecTask(ctx, specTask)

	// Build the prompt using the shared function from services package
	message := services.BuildApprovalInstructionPrompt(specTask, branchName, baseBranch, guidelines, primaryRepoName)

	_, err := s.sendMessageToSpecTaskAgent(ctx, specTask, message, "")
	if err != nil {
		return err
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("branch_name", branchName).
		Msg("✅ Sent approval instruction to agent via WebSocket")

	return nil
}

// getGuidelinesForSpecTask fetches concatenated organization + project guidelines
func (s *HelixAPIServer) getGuidelinesForSpecTask(ctx context.Context, task *types.SpecTask) string {
	if task.ProjectID == "" {
		return ""
	}

	project, err := s.Store.GetProject(ctx, task.ProjectID)
	if err != nil || project == nil {
		return ""
	}

	guidelines := ""

	// Get organization guidelines
	if project.OrganizationID != "" {
		org, err := s.Store.GetOrganization(ctx, &store.GetOrganizationQuery{ID: project.OrganizationID})
		if err == nil && org != nil && org.Guidelines != "" {
			guidelines = org.Guidelines
		}
	}

	// Append project guidelines
	if project.Guidelines != "" {
		if guidelines != "" {
			guidelines += "\n\n---\n\n"
		}
		guidelines += project.Guidelines
	}

	return guidelines
}
