package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, types.ActionGet); err != nil {
		log.Warn().
			Err(err).
			Str("user_id", user.ID).
			Str("project_id", specTask.ProjectID).
			Msg("User not authorized to read spec task design reviews")
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	reviews, err := s.Store.ListSpecTaskDesignReviews(ctx, specTaskID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := &types.SpecTaskDesignReviewListResponse{
		Reviews: reviews,
		Total:   len(reviews),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, "read"); err != nil {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, "update"); err != nil {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
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

		specTask.Status = types.TaskStatusSpecApproved
		specTask.SpecApprovedBy = user.ID
		specTask.SpecApprovedAt = &now

		if err := s.Store.UpdateSpecTask(ctx, specTask); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, "update"); err != nil {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, "read"); err != nil {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
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

	if err := s.authorizeUserToResource(ctx, user, "", specTask.ProjectID, types.ResourceProject, "update"); err != nil {
		http.Error(w, "Not authorized", http.StatusForbidden)
		return
	}

	comment, err := s.Store.GetSpecTaskDesignReviewComment(ctx, commentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	comment.Resolved = true
	comment.ResolvedBy = user.ID
	now := time.Now()
	comment.ResolvedAt = &now

	if err := s.Store.UpdateSpecTaskDesignReviewComment(ctx, comment); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(comment)
}
