package store

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// Design Review methods

func (s *PostgresStore) CreateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error {
	return s.gdb.WithContext(ctx).Create(review).Error
}

func (s *PostgresStore) GetSpecTaskDesignReview(ctx context.Context, id string) (*types.SpecTaskDesignReview, error) {
	var review types.SpecTaskDesignReview
	err := s.gdb.WithContext(ctx).
		Preload("Comments").
		Preload("SpecTask").
		First(&review, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &review, nil
}

func (s *PostgresStore) UpdateSpecTaskDesignReview(ctx context.Context, review *types.SpecTaskDesignReview) error {
	return s.gdb.WithContext(ctx).Save(review).Error
}

func (s *PostgresStore) DeleteSpecTaskDesignReview(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Delete(&types.SpecTaskDesignReview{}, "id = ?", id).Error
}

func (s *PostgresStore) ListSpecTaskDesignReviews(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReview, error) {
	var reviews []types.SpecTaskDesignReview
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		Order("created_at DESC").
		Find(&reviews).Error
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

func (s *PostgresStore) GetLatestDesignReview(ctx context.Context, specTaskID string) (*types.SpecTaskDesignReview, error) {
	var review types.SpecTaskDesignReview
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		Order("created_at DESC").
		First(&review).Error
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// Design Review Comment methods

func (s *PostgresStore) CreateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error {
	return s.gdb.WithContext(ctx).Create(comment).Error
}

func (s *PostgresStore) GetSpecTaskDesignReviewComment(ctx context.Context, id string) (*types.SpecTaskDesignReviewComment, error) {
	var comment types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Preload("Replies").
		Preload("Review").
		First(&comment, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

func (s *PostgresStore) UpdateSpecTaskDesignReviewComment(ctx context.Context, comment *types.SpecTaskDesignReviewComment) error {
	return s.gdb.WithContext(ctx).Save(comment).Error
}

func (s *PostgresStore) DeleteSpecTaskDesignReviewComment(ctx context.Context, id string) error {
	return s.gdb.WithContext(ctx).Delete(&types.SpecTaskDesignReviewComment{}, "id = ?", id).Error
}

func (s *PostgresStore) ListSpecTaskDesignReviewComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error) {
	var comments []types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Where("review_id = ?", reviewID).
		Preload("Replies").
		Order("created_at ASC").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (s *PostgresStore) ListUnresolvedComments(ctx context.Context, reviewID string) ([]types.SpecTaskDesignReviewComment, error) {
	var comments []types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Where("review_id = ? AND resolved = ?", reviewID, false).
		Preload("Replies").
		Order("created_at ASC").
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// Design Review Comment Reply methods

func (s *PostgresStore) CreateSpecTaskDesignReviewCommentReply(ctx context.Context, reply *types.SpecTaskDesignReviewCommentReply) error {
	return s.gdb.WithContext(ctx).Create(reply).Error
}

func (s *PostgresStore) GetSpecTaskDesignReviewCommentReply(ctx context.Context, id string) (*types.SpecTaskDesignReviewCommentReply, error) {
	var reply types.SpecTaskDesignReviewCommentReply
	err := s.gdb.WithContext(ctx).
		Preload("Comment").
		First(&reply, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &reply, nil
}

func (s *PostgresStore) ListSpecTaskDesignReviewCommentReplies(ctx context.Context, commentID string) ([]types.SpecTaskDesignReviewCommentReply, error) {
	var replies []types.SpecTaskDesignReviewCommentReply
	err := s.gdb.WithContext(ctx).
		Where("comment_id = ?", commentID).
		Order("created_at ASC").
		Find(&replies).Error
	if err != nil {
		return nil, err
	}
	return replies, nil
}

// Git Push Event methods

func (s *PostgresStore) CreateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error {
	return s.gdb.WithContext(ctx).Create(event).Error
}

func (s *PostgresStore) GetSpecTaskGitPushEvent(ctx context.Context, id string) (*types.SpecTaskGitPushEvent, error) {
	var event types.SpecTaskGitPushEvent
	err := s.gdb.WithContext(ctx).First(&event, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *PostgresStore) GetSpecTaskGitPushEventByCommit(ctx context.Context, specTaskID, commitHash string) (*types.SpecTaskGitPushEvent, error) {
	var event types.SpecTaskGitPushEvent
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ? AND commit_hash = ?", specTaskID, commitHash).
		First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

func (s *PostgresStore) UpdateSpecTaskGitPushEvent(ctx context.Context, event *types.SpecTaskGitPushEvent) error {
	return s.gdb.WithContext(ctx).Save(event).Error
}

func (s *PostgresStore) ListSpecTaskGitPushEvents(ctx context.Context, specTaskID string) ([]types.SpecTaskGitPushEvent, error) {
	var events []types.SpecTaskGitPushEvent
	err := s.gdb.WithContext(ctx).
		Where("spec_task_id = ?", specTaskID).
		Order("pushed_at DESC").
		Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

func (s *PostgresStore) ListUnprocessedGitPushEvents(ctx context.Context) ([]types.SpecTaskGitPushEvent, error) {
	var events []types.SpecTaskGitPushEvent
	err := s.gdb.WithContext(ctx).
		Where("processed = ?", false).
		Order("pushed_at ASC").
		Limit(100). // Process up to 100 at a time
		Find(&events).Error
	if err != nil {
		return nil, err
	}
	return events, nil
}

// Helper method to check if spec task needs review
func (s *PostgresStore) SpecTaskNeedsReview(ctx context.Context, specTaskID string) (bool, error) {
	var count int64
	err := s.gdb.WithContext(ctx).
		Model(&types.SpecTaskDesignReview{}).
		Where("spec_task_id = ? AND status IN (?)", specTaskID, []types.SpecTaskDesignReviewStatus{
			types.SpecTaskDesignReviewStatusPending,
			types.SpecTaskDesignReviewStatusInReview,
		}).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// Helper to get review with all comments and replies
func (s *PostgresStore) GetSpecTaskDesignReviewWithComments(ctx context.Context, reviewID string) (*types.SpecTaskDesignReview, error) {
	var review types.SpecTaskDesignReview
	err := s.gdb.WithContext(ctx).
		Preload("Comments.Replies").
		Preload("SpecTask").
		First(&review, "id = ?", reviewID).Error
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// Helper to mark review as approved/rejected
func (s *PostgresStore) ApproveDesignReview(ctx context.Context, reviewID, userID string, overallComment string) error {
	review, err := s.GetSpecTaskDesignReview(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("failed to get review: %w", err)
	}

	review.Status = types.SpecTaskDesignReviewStatusApproved
	review.OverallComment = overallComment

	return s.UpdateSpecTaskDesignReview(ctx, review)
}

func (s *PostgresStore) RequestDesignChanges(ctx context.Context, reviewID, userID string, overallComment string) error {
	review, err := s.GetSpecTaskDesignReview(ctx, reviewID)
	if err != nil {
		return fmt.Errorf("failed to get review: %w", err)
	}

	review.Status = types.SpecTaskDesignReviewStatusChangesRequested
	review.OverallComment = overallComment

	return s.UpdateSpecTaskDesignReview(ctx, review)
}

// Get comment by interaction ID (for linking agent responses)
func (s *PostgresStore) GetCommentByInteractionID(ctx context.Context, interactionID string) (*types.SpecTaskDesignReviewComment, error) {
	var comment types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Where("interaction_id = ?", interactionID).
		First(&comment).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// Get comment by request ID (for linking agent responses via request_id)
func (s *PostgresStore) GetCommentByRequestID(ctx context.Context, requestID string) (*types.SpecTaskDesignReviewComment, error) {
	var comment types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Where("request_id = ?", requestID).
		First(&comment).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// Get all unresolved comments for a spec task (for auto-resolution)
func (s *PostgresStore) GetUnresolvedCommentsForTask(ctx context.Context, specTaskID string) ([]types.SpecTaskDesignReviewComment, error) {
	var comments []types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Joins("JOIN spec_task_design_reviews ON spec_task_design_reviews.id = spec_task_design_review_comments.review_id").
		Where("spec_task_design_reviews.spec_task_id = ? AND spec_task_design_review_comments.resolved = ?", specTaskID, false).
		Find(&comments).Error
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// GetPendingCommentByPlanningSessionID finds a comment that has a pending request_id
// for a given planning session. This is used for response linking after API restart
// when in-memory requestToSessionMapping is lost.
func (s *PostgresStore) GetPendingCommentByPlanningSessionID(ctx context.Context, planningSessionID string) (*types.SpecTaskDesignReviewComment, error) {
	var comment types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Joins("JOIN spec_task_design_reviews ON spec_task_design_reviews.id = spec_task_design_review_comments.review_id").
		Joins("JOIN spec_tasks ON spec_tasks.id = spec_task_design_reviews.spec_task_id").
		Where("spec_tasks.planning_session_id = ? AND spec_task_design_review_comments.request_id IS NOT NULL AND spec_task_design_review_comments.request_id != ''", planningSessionID).
		Order("spec_task_design_review_comments.created_at DESC").
		First(&comment).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// GetNextQueuedCommentForSession finds the next comment queued for agent processing.
// This is the PRIMARY mechanism for the comment queue (database-backed, restart-resilient).
// A comment is queued if:
// - queued_at IS NOT NULL (was submitted for processing)
// - request_id IS NULL OR '' (not currently being processed)
// - agent_response IS NULL OR '' (no response received yet)
// Returns oldest queued comment (FIFO order by queued_at).
func (s *PostgresStore) GetNextQueuedCommentForSession(ctx context.Context, planningSessionID string) (*types.SpecTaskDesignReviewComment, error) {
	var comment types.SpecTaskDesignReviewComment
	err := s.gdb.WithContext(ctx).
		Joins("JOIN spec_task_design_reviews ON spec_task_design_reviews.id = spec_task_design_review_comments.review_id").
		Joins("JOIN spec_tasks ON spec_tasks.id = spec_task_design_reviews.spec_task_id").
		Where("spec_tasks.planning_session_id = ?", planningSessionID).
		Where("spec_task_design_review_comments.queued_at IS NOT NULL").
		Where("(spec_task_design_review_comments.request_id IS NULL OR spec_task_design_review_comments.request_id = '')").
		Where("(spec_task_design_review_comments.agent_response IS NULL OR spec_task_design_review_comments.agent_response = '')").
		Order("spec_task_design_review_comments.queued_at ASC").
		First(&comment).Error
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

// IsCommentBeingProcessedForSession checks if there's a comment currently being processed
// (has request_id set) for the given session. Used to prevent concurrent processing.
func (s *PostgresStore) IsCommentBeingProcessedForSession(ctx context.Context, planningSessionID string) (bool, error) {
	var count int64
	err := s.gdb.WithContext(ctx).
		Model(&types.SpecTaskDesignReviewComment{}).
		Joins("JOIN spec_task_design_reviews ON spec_task_design_reviews.id = spec_task_design_review_comments.review_id").
		Joins("JOIN spec_tasks ON spec_tasks.id = spec_task_design_reviews.spec_task_id").
		Where("spec_tasks.planning_session_id = ?", planningSessionID).
		Where("spec_task_design_review_comments.request_id IS NOT NULL AND spec_task_design_review_comments.request_id != ''").
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetSessionsWithPendingComments returns all planning session IDs that have comments waiting to be processed.
// Used on startup to resume processing of queued comments.
func (s *PostgresStore) GetSessionsWithPendingComments(ctx context.Context) ([]string, error) {
	var sessionIDs []string
	err := s.gdb.WithContext(ctx).
		Model(&types.SpecTaskDesignReviewComment{}).
		Select("DISTINCT spec_tasks.planning_session_id").
		Joins("JOIN spec_task_design_reviews ON spec_task_design_reviews.id = spec_task_design_review_comments.review_id").
		Joins("JOIN spec_tasks ON spec_tasks.id = spec_task_design_reviews.spec_task_id").
		Where("spec_task_design_review_comments.queued_at IS NOT NULL").
		Where("spec_tasks.planning_session_id IS NOT NULL AND spec_tasks.planning_session_id != ''").
		Pluck("spec_tasks.planning_session_id", &sessionIDs).Error
	if err != nil {
		return nil, err
	}
	return sessionIDs, nil
}

// ResetStuckComments clears RequestID on comments that were mid-processing when server crashed.
// These are comments with RequestID set but no AgentResponse (stuck in processing state).
// Returns the number of comments reset.
func (s *PostgresStore) ResetStuckComments(ctx context.Context) (int64, error) {
	result := s.gdb.WithContext(ctx).
		Model(&types.SpecTaskDesignReviewComment{}).
		Where("request_id IS NOT NULL AND request_id != ''").
		Where("(agent_response IS NULL OR agent_response = '')").
		Updates(map[string]interface{}{
			"request_id": "",
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}
