package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
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

// CommentTimerNoResponseMessage is the AgentResponse stamped onto a comment
// when the 2-minute response timer fires without the agent producing any
// content. It must be a stable string because finalizeCommentResponse keys
// off it to recognise a stale timer-stamp and overwrite it with a late
// real response.
const CommentTimerNoResponseMessage = "[Agent did not respond - try sending your comment again]"

// commentResponseTimeout is how long we wait for the agent to respond to a
// queued review comment before the backstop timer fires. The timer is a
// best-effort safety net behind finalizeCommentResponse (which fires on the
// message_completed event); see handleCommentTimeout for the decision tree.
const commentResponseTimeout = 2 * time.Minute

// Design Review Handlers - Simple versions

// ResumeCommentQueueProcessing resumes processing of any queued comments after server restart.
// This should be called during server startup.
// It:
// 1. Resets any comments stuck in "processing" state (RequestID set but no response)
// 2. Triggers processing for all sessions that have pending comments
func (s *HelixAPIServer) ResumeCommentQueueProcessing(ctx context.Context) {
	log.Info().Msg("🔄 Resuming comment queue processing after startup...")

	// Find all sessions with pending comments up front — used both for terminal
	// reconciliation and for triggering processing.
	sessionIDs, err := s.Store.GetSessionsWithPendingComments(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get sessions with pending comments")
		return
	}

	// Step 1a: Reconcile comments whose agent interaction already COMPLETED but
	// were never finalized (their message_completed never mapped back). These must
	// be finalized — not blindly reset — otherwise the bulk reset below would clear
	// their request_id and the queue would re-send an already-answered comment.
	for _, sessionID := range sessionIDs {
		s.reconcileStuckInFlightComment(ctx, sessionID)
	}

	// Step 1b: Reset any comments still stuck mid-processing (request_id set, no
	// response, interaction NOT terminal — a genuine crash mid-flight). Clearing
	// request_id lets them be re-sent on the next processing pass.
	resetCount, err := s.Store.ResetStuckComments(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to reset stuck comments")
	} else if resetCount > 0 {
		log.Info().Int64("count", resetCount).Msg("✅ Reset stuck comments (were mid-processing during crash)")
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
				// Backfill synchronously so the response includes the review
				s.backfillDesignReviewFromGit(ctx, specTaskID, repo)
				// Re-fetch reviews after backfill
				reviews, err = s.Store.ListSpecTaskDesignReviews(ctx, specTaskID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
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

	jsonBytes, err := json.Marshal(response)
	if err != nil {
		http.Error(w, "failed to encode response", http.StatusInternalServerError)
		return
	}

	h := fnv.New64a()
	h.Write(jsonBytes)
	etag := fmt.Sprintf(`"%x"`, h.Sum64())

	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache, must-revalidate")

	if match := r.Header.Get("If-None-Match"); match == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonBytes) //nolint:errcheck
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
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	var req types.SpecTaskDesignReviewSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Detach from request context so DB mutations complete even if client disconnects
	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()

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
		if review.Status == types.SpecTaskDesignReviewStatusApproved {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(review)
			return
		}

		review.Status = types.SpecTaskDesignReviewStatusApproved
		now := time.Now()
		review.ApprovedAt = &now
		review.OverallComment = req.OverallComment

		switch specTask.Status {
		case types.TaskStatusSpecReview, types.TaskStatusSpecRevision, types.TaskStatusSpecGeneration:
			// Before advancing to implementation, validate the approver has
			// GitHub OAuth so their credentials can be used for commits and
			// push. Mirrors the check in approveSpecs/approveImplementation —
			// the UI goes through this endpoint, so omitting the check here
			// lets an approver without OAuth silently drive the task to
			// implementation and commits would then fall back to the creator.
			if project, projErr := s.Store.GetProject(ctx, specTask.ProjectID); projErr == nil && project.DefaultRepoID != "" {
				if repo, repoErr := s.Store.GetGitRepository(ctx, project.DefaultRepoID); repoErr == nil {
					if err := s.gitRepositoryService.ValidateUserGitHubOAuth(ctx, repo, user.ID); err != nil {
						var oauthErr *services.OAuthRequiredError
						if errors.As(err, &oauthErr) {
							writeResponse(w, map[string]interface{}{
								"error":         "oauth_required",
								"message":       oauthErr.Error(),
								"provider_type": oauthErr.ProviderType,
							}, http.StatusUnprocessableEntity)
							return
						}
						log.Warn().Err(err).Str("task_id", specTask.ID).
							Msg("Non-OAuthRequired error validating approver OAuth at design-review submit; proceeding with approval")
					}
				}
			}

			specTask.Status = types.TaskStatusSpecApproved
			specTask.SpecApprovedBy = user.ID
			specTask.SpecApprovedAt = &now
			specTask.StatusUpdatedAt = &now
			specTask.SpecApproval = &types.SpecApprovalResponse{
				TaskID:     specTask.ID,
				Approved:   true,
				ApprovedBy: user.ID,
				ApprovedAt: now,
				Comments:   req.OverallComment,
			}

			if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			if s.auditLogService != nil {
				s.auditLogService.LogTaskApproved(ctx, specTask, user.ID, user.Email)
			}

			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				if err := s.specDrivenTaskService.ApproveSpecs(context.Background(), specTask); err != nil {
					log.Error().
						Err(err).
						Str("spec_task_id", specTask.ID).
						Str("review_id", review.ID).
						Msg("[DesignReview] Failed to process spec approval (orchestrator will retry)")
				}
			}()
		default:
			log.Info().
				Str("spec_task_id", specTask.ID).
				Str("status", string(specTask.Status)).
				Msg("[DesignReview] Task already past spec phase, updating review only")
		}

	case "request_changes":
		review.Status = types.SpecTaskDesignReviewStatusChangesRequested
		now := time.Now()
		review.RejectedAt = &now
		review.OverallComment = req.OverallComment

		specTask.Status = types.TaskStatusSpecRevision
		specTask.StatusUpdatedAt = &now
		specTask.SpecRevisionCount++

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Notify agent of requested changes via WebSocket.
		// interrupt=true: reviewer-driven feedback should preempt any in-flight agent turn,
		// matching the comment-queue semantic.
		message := services.BuildRevisionInstructionPrompt(specTask, req.OverallComment)
		_, _, err = s.sendMessageToSpecTaskAgent(ctx, specTask, message, user.ID, true)
		if err != nil {
			log.Error().
				Err(err).
				Str("spec_task_id", specTask.ID).
				Str("review_id", review.ID).
				Msg("[DesignReview] Failed to notify agent of requested changes")
			// Don't fail the request - the review state is already updated
			// Agent can see the feedback when they check the review
		} else {
			log.Info().
				Str("spec_task_id", specTask.ID).
				Str("review_id", review.ID).
				Int("revision_count", specTask.SpecRevisionCount).
				Msg("[DesignReview] Changes requested, agent notified via WebSocket")
		}
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
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	reviewID := vars["review_id"]

	var req types.SpecTaskDesignReviewCommentCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Detach from request context so DB mutations complete even if client disconnects
	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()

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
	user := getRequestUser(r)
	vars := mux.Vars(r)
	specTaskID := vars["spec_task_id"]
	commentID := vars["comment_id"]

	// Detach from request context so DB mutations complete even if client disconnects
	ctx, cancel := detachContext(r.Context(), 30*time.Second)
	defer cancel()

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
		// A comment is marked in-flight (request_id set). Normally we skip to avoid
		// interleaving agent responses. But a comment can get stuck in-flight forever
		// if its message_completed event never maps back to it — e.g. the agent
		// coalesced several re-sends under a different request_id, the completion was
		// missed/duplicated, or a restart lost the in-memory mapping. In that case the
		// linked interaction is already terminal but finalizeCommentResponse never ran,
		// so request_id stays set and blocks the whole session's comment queue.
		//
		// Reconcile before skipping: if the in-flight comment's interaction is terminal,
		// finalize it now (copies the response, clears the marker, advances the queue).
		if s.reconcileStuckInFlightComment(ctx, sessionID) {
			// finalizeCommentResponse already triggered the next queue item.
			return
		}
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

	// Now actually send the comment to the agent.
	// sendCommentToAgentNow → sendMessageToSpecTaskAgent, which auto-starts the dev container
	// and waits up to 90s for the agent to connect if no session is currently active.
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
	s.armCommentTimer(sessionID, comment.ID)

	log.Info().
		Str("session_id", sessionID).
		Str("comment_id", comment.ID).
		Msg("Comment sent to agent, started 2-minute response timeout")
}

// armCommentTimer (re)arms the per-session backstop timer for a comment. It
// cancels any existing timer for the session and schedules handleCommentTimeout
// to run after commentResponseTimeout. A fresh context.Background() is used for
// the timer body because the originating HTTP request context may already be
// cancelled by the time the timer fires.
func (s *HelixAPIServer) armCommentTimer(sessionID, commentID string) {
	s.sessionCommentMutex.Lock()
	defer s.sessionCommentMutex.Unlock()
	if existingTimer := s.sessionCommentTimeout[sessionID]; existingTimer != nil {
		existingTimer.Stop()
	}
	s.sessionCommentTimeout[sessionID] = time.AfterFunc(commentResponseTimeout, func() {
		s.handleCommentTimeout(context.Background(), sessionID, commentID)
	})
}

// reconcileStuckInFlightComment detects a comment that is marked in-flight
// (request_id set) for a session but whose linked agent interaction has already
// reached a terminal state without finalizeCommentResponse ever running. This
// happens when the message_completed event for the turn never maps back to the
// comment's request_id (the agent coalesced several re-sends under a different
// request_id, the completion was missed/duplicated, or a restart lost the
// in-memory mapping). Left alone, such a zombie blocks the session's comment
// queue forever.
//
// If a stuck comment is found and reconciled it is finalized (response copied
// from the interaction, request_id/queued_at cleared, next queued comment
// processed) and true is returned. A genuinely in-flight comment whose
// interaction is still waiting/streaming is left untouched and false is returned.
func (s *HelixAPIServer) reconcileStuckInFlightComment(ctx context.Context, sessionID string) bool {
	inFlight, err := s.Store.GetPendingCommentByPlanningSessionID(ctx, sessionID)
	if err != nil || inFlight == nil {
		return false
	}
	if inFlight.InteractionID == "" {
		// No linked interaction — we can't prove the agent finished, so treat it as
		// genuinely in flight and leave it for the backstop timer / re-send path.
		return false
	}
	interaction, ierr := s.Store.GetInteraction(ctx, inFlight.InteractionID)
	if ierr != nil || interaction == nil {
		return false
	}
	terminal := interaction.State == types.InteractionStateComplete ||
		interaction.State == types.InteractionStateInterrupted ||
		interaction.State == types.InteractionStateError
	if !terminal {
		// Agent is still working on this comment — correct to wait.
		return false
	}
	log.Warn().
		Str("session_id", sessionID).
		Str("comment_id", inFlight.ID).
		Str("interaction_id", inFlight.InteractionID).
		Str("interaction_state", string(interaction.State)).
		Str("request_id", inFlight.RequestID).
		Msg("🩹 [HELIX] Reconciling stuck in-flight comment: interaction is terminal but was never finalized — finalizing now to unblock the queue")
	if err := s.finalizeCommentResponse(ctx, inFlight.RequestID); err != nil {
		log.Error().Err(err).
			Str("comment_id", inFlight.ID).
			Msg("Failed to finalize stuck in-flight comment during reconciliation")
		return false
	}
	return true
}

// handleCommentTimeout is the body of the per-comment 2-minute response timer.
// It runs on the timer's goroutine when the timer fires. It is exposed as a
// method (rather than living inline as a closure) so it can be unit-tested
// without spinning a real time.AfterFunc.
//
// The decision tree is:
//   - If the comment is no longer being processed (RequestID cleared) or
//     already has a real response: nothing to do.
//   - If the linked interaction has any streamed content OR is in a terminal
//     state (complete / interrupted / error): the agent IS responding — skip
//     the error stamp and let finalizeCommentResponse copy the content over
//     when message_completed eventually arrives. This is the fix for the
//     "agent is taking longer than 2 minutes to produce a long answer"
//     false-positive that previously stamped the error onto the comment.
//   - Otherwise the agent genuinely produced nothing: stamp the error so the
//     user sees a clear "try again" signal, then process the next queued
//     comment.
func (s *HelixAPIServer) handleCommentTimeout(ctx context.Context, sessionID, commentID string) {
	log.Warn().
		Str("session_id", sessionID).
		Str("comment_id", commentID).
		Msg("Comment response timeout - agent did not respond within 2 minutes")

	currentComment, fetchErr := s.Store.GetSpecTaskDesignReviewComment(ctx, commentID)
	if fetchErr != nil {
		log.Error().Err(fetchErr).Str("comment_id", commentID).Msg("Failed to fetch comment for timeout check")
		return
	}

	// Already resolved (request done OR a real response landed): nothing to do.
	if currentComment.RequestID == "" || currentComment.AgentResponse != "" {
		return
	}

	// Check whether the agent is actively making progress on the linked
	// interaction. During streaming the response content lives on the
	// interaction row (ResponseMessage / ResponseEntries), not on the comment
	// — comment.AgentResponse is only populated when message_completed fires.
	// So an empty AgentResponse by itself does not prove the agent has been
	// silent.
	if currentComment.InteractionID != "" {
		interaction, ierr := s.Store.GetInteraction(ctx, currentComment.InteractionID)
		if ierr == nil && interaction != nil {
			agentText := types.TextFromInteraction(interaction)
			terminal := interaction.State == types.InteractionStateComplete ||
				interaction.State == types.InteractionStateInterrupted ||
				interaction.State == types.InteractionStateError
			if terminal {
				// The agent is DONE but finalizeCommentResponse never ran — its
				// message_completed didn't map back to this comment (coalesced
				// re-sends, missed/duplicate completion, restart). Don't just defer:
				// finalize here so the comment gets its response and the queue is
				// unblocked. Without this the comment stays in-flight forever and
				// every later comment for this session is silently never delivered.
				log.Warn().
					Str("session_id", sessionID).
					Str("comment_id", commentID).
					Str("interaction_id", interaction.ID).
					Str("interaction_state", string(interaction.State)).
					Int("interaction_response_len", len(agentText)).
					Msg("🩹 Comment timer: interaction is terminal but was never finalized — finalizing now to unblock the queue")
				if err := s.finalizeCommentResponse(ctx, currentComment.RequestID); err != nil {
					log.Error().Err(err).
						Str("comment_id", commentID).
						Str("request_id", currentComment.RequestID).
						Msg("Comment timer: failed to finalize terminal interaction")
				}
				return
			}
			if agentText != "" {
				// Non-terminal but has content. Distinguish a long answer that is
				// still actively streaming (defer + re-check) from one that has
				// stalled mid-stream because the agent died (finalize with what we
				// have, otherwise the queue is blocked forever). The interaction's
				// Updated timestamp tells them apart: a live stream keeps bumping it.
				if time.Since(interaction.Updated) > commentResponseTimeout {
					log.Warn().
						Str("session_id", sessionID).
						Str("comment_id", commentID).
						Str("interaction_id", interaction.ID).
						Time("interaction_updated", interaction.Updated).
						Msg("🩹 Comment timer: interaction stalled mid-stream (no updates for a full window) — finalizing partial response to unblock the queue")
					if err := s.finalizeCommentResponse(ctx, currentComment.RequestID); err != nil {
						log.Error().Err(err).
							Str("comment_id", commentID).
							Msg("Comment timer: failed to finalize stalled interaction")
					}
					return
				}
				// Still streaming — re-arm the timer and re-check rather than
				// deferring indefinitely to a finalize that may never arrive.
				log.Info().
					Str("session_id", sessionID).
					Str("comment_id", commentID).
					Str("interaction_id", interaction.ID).
					Int("interaction_response_len", len(agentText)).
					Msg("⏭️  Comment timer: agent still streaming — re-arming timer to re-check")
				s.armCommentTimer(sessionID, commentID)
				return
			}
		} else if ierr != nil {
			log.Warn().Err(ierr).
				Str("comment_id", commentID).
				Str("interaction_id", currentComment.InteractionID).
				Msg("Comment timer: failed to load linked interaction, falling through to error stamp")
		}
	}

	// Genuine no-response: agent produced nothing, interaction has no content
	// and is still waiting. Surface the error so the user can retry.
	currentComment.AgentResponse = CommentTimerNoResponseMessage
	currentComment.RequestID = ""
	currentComment.QueuedAt = nil
	if updateErr := s.Store.UpdateSpecTaskDesignReviewComment(ctx, currentComment); updateErr != nil {
		log.Error().Err(updateErr).Str("comment_id", commentID).Msg("Failed to update timed-out comment")
		return
	}

	go s.processNextCommentInQueue(ctx, sessionID)
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

	// PlanningSessionID not connected - search for the most recently updated
	// connected session with this spec task ID. A spectask can have multiple
	// Zed threads (sessions), so we pick the most recent to route messages
	// to the thread the agent is most likely actively working in.
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("planning_session_id", specTask.PlanningSessionID).
		Msg("PlanningSessionID not connected, searching for alternate connected session")

	connectedSessions := s.externalAgentWSManager.listConnections()

	var bestSession *types.Session
	var bestSessionConnID string
	for _, conn := range connectedSessions {
		session, err := s.Store.GetSession(ctx, conn.SessionID)
		if err != nil {
			continue
		}
		if session.Metadata.SpecTaskID != specTask.ID {
			continue
		}
		if bestSession == nil || session.Updated.After(bestSession.Updated) {
			bestSession = session
			bestSessionConnID = conn.SessionID
		}
	}

	if bestSession != nil {
		log.Info().
			Str("spec_task_id", specTask.ID).
			Str("found_session_id", bestSessionConnID).
			Str("original_planning_session_id", specTask.PlanningSessionID).
			Time("session_updated", bestSession.Updated).
			Msg("✅ Found most recently updated connected session for spec task")
		return bestSessionConnID, nil
	}

	return "", fmt.Errorf("no WebSocket connection found for spec task %s (tried planning session %s and %d other connected sessions)",
		specTask.ID, specTask.PlanningSessionID, len(connectedSessions))
}

// startDevContainerForSession boots the dev container for any zed_external session,
// whether it belongs to a spec task or is an exploratory project session. It is the
// shared body behind resumeSession, startDevContainerForSpecTask, and the auto-wake
// path (autoStartDevContainerForSession). Project context is resolved in priority order:
//
//  1. session.Metadata.SpecTaskID — load the spec task, take ProjectID/OrganizationID from it.
//  2. session.Metadata.ProjectID  — exploratory zed_external session.
//  3. session.ProjectID           — legacy session row.
//
// Returns an error only on real start failures. If no project context is available
// (no spec task AND no project ID anywhere), returns nil after logging — the caller's
// persisted Waiting interaction will simply remain queued; we cannot invent project config.
func (s *HelixAPIServer) startDevContainerForSession(ctx context.Context, session *types.Session) error {
	if session == nil {
		return fmt.Errorf("startDevContainerForSession: session is nil")
	}
	if s.externalAgentExecutor == nil {
		return fmt.Errorf("external agent executor not available")
	}

	agent := &types.DesktopAgent{
		SessionID:   session.ID,
		UserID:      session.Owner,
		Input:       "Resume session",
		ProjectPath: "workspace",
	}

	// Resolve project context. Priority: spec task → session.Metadata.ProjectID → session.ProjectID.
	specTaskID := session.Metadata.SpecTaskID
	if specTaskID != "" {
		specTask, err := s.Store.GetSpecTask(ctx, specTaskID)
		if err != nil {
			log.Warn().Err(err).
				Str("spec_task_id", specTaskID).
				Str("session_id", session.ID).
				Msg("startDevContainerForSession: failed to load spec task, falling back to session metadata")
		} else if specTask != nil {
			agent.SpecTaskID = specTask.ID
			agent.ProjectID = specTask.ProjectID
			agent.OrganizationID = specTask.OrganizationID
		}
	}
	if agent.ProjectID == "" && session.Metadata.ProjectID != "" {
		agent.ProjectID = session.Metadata.ProjectID
		agent.OrganizationID = session.OrganizationID
	}
	if agent.ProjectID == "" && session.ProjectID != "" {
		agent.ProjectID = session.ProjectID
		agent.OrganizationID = session.OrganizationID
	}

	if agent.ProjectID == "" {
		log.Info().
			Str("session_id", session.ID).
			Str("agent_type", session.Metadata.AgentType).
			Msg("startDevContainerForSession: no project context (no spec task, no project ID) — cannot auto-start")
		return nil
	}

	// Load project repositories.
	if err := s.attachProjectContext(ctx, agent, agent.ProjectID); err != nil {
		return fmt.Errorf("attach project context: %w", err)
	}

	// Get display settings from app config.
	if session.ParentApp != "" {
		app, err := s.Controller.Options.Store.GetApp(ctx, session.ParentApp)
		if err == nil && app != nil && app.Config.Helix.ExternalAgentConfig != nil {
			width, height := app.Config.Helix.ExternalAgentConfig.GetEffectiveResolution()
			agent.DisplayWidth = width
			agent.DisplayHeight = height
			if app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate > 0 {
				agent.DisplayRefreshRate = app.Config.Helix.ExternalAgentConfig.DisplayRefreshRate
			}
			agent.Resolution = app.Config.Helix.ExternalAgentConfig.Resolution
			agent.ZoomLevel = app.Config.Helix.ExternalAgentConfig.GetEffectiveZoomLevel()
			agent.DesktopType = app.Config.Helix.ExternalAgentConfig.GetEffectiveDesktopType()
		}
	}

	// Set up the OnBeforeCreate hook to add API tokens inside the session lock.
	// This prevents a race where StopDesktop revokes the key between token
	// creation and container creation — the hook runs inside StartDesktop's
	// per-session lock, which is also held by StopDesktop during key revocation.
	ownerID := session.Owner
	agent.OnBeforeCreate = func(hookCtx context.Context, a *types.DesktopAgent) error {
		return s.addUserAPITokenToAgent(hookCtx, a, ownerID)
	}

	log.Info().
		Str("session_id", session.ID).
		Str("spec_task_id", agent.SpecTaskID).
		Str("project_id", agent.ProjectID).
		Msg("Auto-starting dev container for session (backend-initiated resume)")

	response, err := s.externalAgentExecutor.StartDesktop(ctx, agent)
	if err != nil {
		return fmt.Errorf("failed to start dev container: %w", err)
	}

	// Re-fetch session and update metadata.
	refetched, err := s.Store.GetSession(ctx, session.ID)
	if err == nil && refetched != nil {
		if response.DevContainerID != "" {
			refetched.Metadata.DevContainerID = response.DevContainerID
		}
		refetched.Metadata.PausedScreenshotPath = ""
		if _, err := s.Store.UpdateSession(ctx, *refetched); err != nil {
			log.Warn().Err(err).Str("session_id", refetched.ID).Msg("Failed to update session metadata after auto-start")
		}
	}

	log.Info().
		Str("session_id", session.ID).
		Str("spec_task_id", agent.SpecTaskID).
		Msg("✅ Dev container auto-started, agent will reconnect via WebSocket")

	return nil
}

// startDevContainerForSpecTask is a thin wrapper that loads the spec task's planning
// session, then delegates to startDevContainerForSession. Kept for callers that have
// a SpecTask in hand but not the session.
func (s *HelixAPIServer) startDevContainerForSpecTask(ctx context.Context, specTask *types.SpecTask) error {
	if specTask.PlanningSessionID == "" {
		return fmt.Errorf("spec task %s has no planning session ID", specTask.ID)
	}
	session, err := s.Store.GetSession(ctx, specTask.PlanningSessionID)
	if err != nil {
		return fmt.Errorf("failed to get planning session %s: %w", specTask.PlanningSessionID, err)
	}
	return s.startDevContainerForSession(ctx, session)
}

// sendCommentToAgentNow actually sends a comment to the agent (called from queue processor)
func (s *HelixAPIServer) sendCommentToAgentNow(
	ctx context.Context,
	specTask *types.SpecTask,
	comment *types.SpecTaskDesignReviewComment,
) error {
	// Build prompt for agent using the shared helper
	promptText := services.BuildCommentPrompt(specTask, comment)

	// Send via the unified helper, notifying the commenter of responses.
	// interactionID is returned directly — avoids the fragile session-based queue
	// lookup that breaks when the agent's live session differs from PlanningSessionID.
	// That divergence comes from a user spinning up a separate thread/session in the
	// Zed UI (handleUserCreatedThread); auto-compaction, by contrast, now summarises
	// in-place within the same thread and does NOT fork a new session (older Zed builds
	// did — hence this note). In practice the desktop's anchor connection is the
	// planning session, so sends still resolve to PlanningSessionID.
	// interrupt=true: a design-review comment is reactive feedback that should preempt
	// any in-flight agent turn so the latest input takes priority over stale work.
	requestID, interactionID, err := s.sendMessageToSpecTaskAgent(ctx, specTask, promptText, comment.CommentedBy, true)
	if err != nil {
		log.Error().
			Err(err).
			Str("spec_task_id", specTask.ID).
			Str("comment_id", comment.ID).
			Msg("Failed to send comment to agent via websocket")
		return err
	}

	// Store both IDs on the comment: requestID links to message_completed for finalization,
	// interactionID lets finalizeCommentResponse copy the streamed response at completion time.
	comment.RequestID = requestID
	comment.InteractionID = interactionID

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
	comment.AgentResponse = types.TextFromInteraction(interaction)
	comment.AgentResponseEntries = interaction.ResponseEntries
	now := time.Now()
	comment.AgentResponseAt = &now

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		return fmt.Errorf("failed to update comment with agent response: %w", err)
	}

	log.Info().
		Str("comment_id", comment.ID).
		Str("interaction_id", interaction.ID).
		Int("response_length", len(comment.AgentResponse)).
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

	// Use targeted update that only modifies agent_response fields
	// This prevents race conditions where streaming updates overwrite resolution status set by git hooks
	now := time.Now()
	if err := s.Store.UpdateCommentAgentResponse(ctx, comment.ID, responseContent, &now); err != nil {
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

	// If the comment doesn't have a real AgentResponse yet, try to populate it
	// from the interaction. The message_added events update the interaction's
	// ResponseMessage but not the comment's AgentResponse directly — we copy
	// it over at finalization time.
	//
	// We also treat the literal timer-stamped error string as "no real
	// response" so a late message_completed can repair a comment that the
	// 2-minute timer had pessimistically marked as failed. Without this, the
	// timer error sticks even when the agent later delivers a perfectly good
	// answer.
	needsPopulation := comment.AgentResponse == "" || comment.AgentResponse == CommentTimerNoResponseMessage
	hadStaleTimerError := comment.AgentResponse == CommentTimerNoResponseMessage
	if needsPopulation && comment.InteractionID != "" {
		interaction, interactionErr := s.Store.GetInteraction(ctx, comment.InteractionID)
		if interactionErr == nil {
			text := types.TextFromInteraction(interaction)
			if text != "" {
				comment.AgentResponse = text
				comment.AgentResponseEntries = interaction.ResponseEntries
				now := time.Now()
				comment.AgentResponseAt = &now
				if hadStaleTimerError {
					log.Warn().
						Str("comment_id", comment.ID).
						Str("interaction_id", comment.InteractionID).
						Int("response_length", len(text)).
						Msg("🔁 [HELIX] Overwriting stale 'agent did not respond' timer-stamp with real agent response from interaction")
				} else {
					log.Info().
						Str("comment_id", comment.ID).
						Str("interaction_id", comment.InteractionID).
						Int("response_length", len(text)).
						Msg("📝 [HELIX] Populated comment AgentResponse from interaction at finalization")
				}
			}
		}
	}

	// If we still don't have a real response AND we have a session, try the latest interaction
	if comment.AgentResponse == "" || comment.AgentResponse == CommentTimerNoResponseMessage {
		s.populateAgentResponseFromSession(ctx, comment)
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

	// Clean up sessionToCommenterMapping now that response is complete
	s.contextMappingsMutex.Lock()
	if s.sessionToCommenterMapping != nil {
		delete(s.sessionToCommenterMapping, sessionID)
		log.Debug().Str("session_id", sessionID).Msg("🧹 [HELIX] Cleaned up sessionToCommenterMapping")
	}
	s.contextMappingsMutex.Unlock()

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

// populateAgentResponseFromSession is a fallback that finds the agent's response
// from the session's latest interaction when streaming didn't populate it directly.
func (s *HelixAPIServer) populateAgentResponseFromSession(ctx context.Context, comment *types.SpecTaskDesignReviewComment) {
	review, err := s.Store.GetSpecTaskDesignReview(ctx, comment.ReviewID)
	if err != nil {
		return
	}
	specTask, err := s.Store.GetSpecTask(ctx, review.SpecTaskID)
	if err != nil || specTask.PlanningSessionID == "" {
		return
	}
	session, err := s.Store.GetSession(ctx, specTask.PlanningSessionID)
	if err != nil || len(session.Interactions) == 0 {
		return
	}
	// Walk backwards to find the most recent interaction with a response
	for i := len(session.Interactions) - 1; i >= 0; i-- {
		text := types.TextFromInteraction(session.Interactions[i])
		if text != "" {
			comment.AgentResponse = text
			comment.AgentResponseEntries = session.Interactions[i].ResponseEntries
			now := time.Now()
			comment.AgentResponseAt = &now
			log.Info().
				Str("comment_id", comment.ID).
				Str("interaction_id", session.Interactions[i].ID).
				Int("response_length", len(text)).
				Msg("📝 [HELIX] Populated comment AgentResponse from latest session interaction")
			return
		}
	}
}

// backfillDesignReviewFromGit creates a design review from the current state of helix-specs branch
// Used for self-healing when a task is in spec_review but has no review record
func (s *HelixAPIServer) backfillDesignReviewFromGit(ctx context.Context, specTaskID string, repo *types.GitRepository) {
	log.Info().
		Str("spec_task_id", specTaskID).
		Msg("Backfilling design review from git")

	// No need to sync from upstream - helix-specs is only written by Helix agents,
	// so our middle repo always has the latest data.

	repoPath := repo.LocalPath

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

// backfillCommentLinkageForPrompt links a design-review comment to the
// interaction the queue just created for its prompt. The comment path enqueues
// with comment.PromptID set but does not know the interaction/request id until
// dispatch; this sets comment.RequestID and comment.InteractionID (both equal
// the interaction id on the queue path) so the existing comment finalize /
// streaming / timeout / reconcile machinery — which keys off those fields —
// works unchanged. No-op for non-comment prompts (the lookup returns nothing).
func (s *HelixAPIServer) backfillCommentLinkageForPrompt(ctx context.Context, promptID, requestID, interactionID string) {
	if promptID == "" {
		return
	}
	comment, err := s.Store.GetCommentByPromptID(ctx, promptID)
	if err != nil || comment == nil {
		// Normal for non-comment prompts (CI, push, bots, user sends).
		return
	}
	if comment.RequestID == requestID && comment.InteractionID == interactionID {
		return
	}
	comment.RequestID = requestID
	comment.InteractionID = interactionID
	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		log.Error().Err(err).
			Str("comment_id", comment.ID).
			Str("prompt_id", promptID).
			Str("request_id", requestID).
			Msg("Failed to backfill comment linkage at dispatch — comment response may not finalize")
		return
	}
	log.Info().
		Str("comment_id", comment.ID).
		Str("prompt_id", promptID).
		Str("request_id", requestID).
		Str("interaction_id", interactionID).
		Msg("🔗 [HELIX] Backfilled design-review comment linkage from queued prompt dispatch")
}

// enqueueSpecTaskAgentMessage is the spec-task-shaped SpecTaskMessageEnqueuer:
// it enqueues onto the session-scoped prompt queue for the task's canonical
// planning session. Delivery is async (the poller dispatches when idle, or
// cancels-then-sends for interrupt=true) — this is the single sender path that
// replaces the old immediate direct dispatch.
func (s *HelixAPIServer) enqueueSpecTaskAgentMessage(ctx context.Context, task *types.SpecTask, message string, interrupt bool, notifyUserID string) error {
	if task.PlanningSessionID == "" {
		return fmt.Errorf("cannot enqueue message: spec task %s has no planning session", task.ID)
	}
	_, err := s.enqueueAgentMessage(ctx, task.PlanningSessionID, message, interrupt, notifyUserID, task.ID)
	return err
}

// sendMessageToSpecTaskAgent is the unified helper for sending messages to spec task agents via WebSocket
// It handles: finding connected session, generating request ID, setting up response routing, and sending.
// If no session is connected, sendChatMessageToExternalAgent persists the interaction; the no-WS path
// triggers autoStartDevContainerForSession and pickupWaitingInteraction delivers on reconnect.
// Returns (requestID, interactionID, error). Both IDs are needed by callers that track comment responses.
//
// interrupt=true tells the agent to cancel its current turn before processing this message. Use it for
// reactive feedback (design-review comments, request-changes flows). Use false for system-driven
// instructions (approval kickoff, post-merge push/rebase) that should respect the agent's queue.
func (s *HelixAPIServer) sendMessageToSpecTaskAgent(
	ctx context.Context,
	specTask *types.SpecTask,
	message string,
	notifyUserID string, // Optional: user to notify of responses (e.g., commenter). Empty = no extra notification
	interrupt bool,
) (string, string, error) {
	// Find a connected session for this spec task, falling back to PlanningSessionID.
	// If no session is connected, sendChatMessageToExternalAgent will still create
	// the interaction. sendCommandToExternalAgent will fail and trigger auto-start;
	// pickupWaitingInteraction delivers the message when the agent reconnects.
	sessionID, err := s.findConnectedSessionForSpecTask(ctx, specTask)
	if err != nil {
		if specTask.PlanningSessionID == "" {
			return "", "", fmt.Errorf("no connected session and no planning session ID: %w", err)
		}
		log.Info().
			Str("spec_task_id", specTask.ID).
			Str("planning_session_id", specTask.PlanningSessionID).
			Msg("No connected session, falling back to planning session ID — auto-start will be triggered on send")
		sessionID = specTask.PlanningSessionID
	}

	return s.sendMessageToSession(ctx, sessionID, message, notifyUserID, interrupt)
}

// sendMessageToSession is the session-scoped helper for delivering a message to an external agent.
// It generates a request ID, optionally registers a notify-user mapping for response routing, and
// calls sendChatMessageToExternalAgent — which persists a Waiting interaction even when no agent
// WebSocket is connected. If the WS is absent, ErrNoExternalAgentWS is returned wrapped: callers
// should treat that as "queued, will deliver on reconnect" via pickupWaitingInteraction, not a
// hard failure. The interactionID is returned even in that case so the caller can correlate
// responses on /api/v1/ws/user.
func (s *HelixAPIServer) sendMessageToSession(
	ctx context.Context,
	sessionID string,
	message string,
	notifyUserID string,
	interrupt bool,
) (string, string, error) {
	_ = ctx // session lookup is delegated to sendChatMessageToExternalAgent

	requestID := "req_" + system.GenerateUUID()

	if notifyUserID != "" {
		s.contextMappingsMutex.Lock()
		if s.requestToCommenterMapping == nil {
			s.requestToCommenterMapping = make(map[string]string)
		}
		s.requestToCommenterMapping[requestID] = notifyUserID
		if s.sessionToCommenterMapping == nil {
			s.sessionToCommenterMapping = make(map[string]string)
		}
		s.sessionToCommenterMapping[sessionID] = notifyUserID
		s.contextMappingsMutex.Unlock()
	}

	interactionID, err := s.sendChatMessageToExternalAgent(sessionID, message, requestID, interrupt)
	if err != nil {
		// ErrNoExternalAgentWS means the interaction was persisted but no WS was connected.
		// pickupWaitingInteraction will deliver it on reconnect — surface as success to
		// the caller, who already has the interactionID for response correlation.
		if errors.Is(err, ErrNoExternalAgentWS) {
			log.Info().
				Str("session_id", sessionID).
				Str("interaction_id", interactionID).
				Str("request_id", requestID).
				Msg("✉️  Queued message for session — no WS connected, will deliver on reconnect")
			return requestID, interactionID, nil
		}

		if notifyUserID != "" {
			s.contextMappingsMutex.Lock()
			delete(s.requestToCommenterMapping, requestID)
			s.contextMappingsMutex.Unlock()
		}
		return "", "", fmt.Errorf("failed to send message via WebSocket: %w", err)
	}

	log.Info().
		Str("session_id", sessionID).
		Str("interaction_id", interactionID).
		Str("request_id", requestID).
		Msg("✅ Sent message to session agent via WebSocket")

	return requestID, interactionID, nil
}

