package services

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// SpecTaskReviewNotifier handles notifying agents about design review feedback
type SpecTaskReviewNotifier struct {
	store store.Store
}

// NewSpecTaskReviewNotifier creates a new review notifier
func NewSpecTaskReviewNotifier(store store.Store) *SpecTaskReviewNotifier {
	return &SpecTaskReviewNotifier{
		store: store,
	}
}

// NotifyAgentOfReviewFeedback sends review feedback to the agent via WebSocket
func (n *SpecTaskReviewNotifier) NotifyAgentOfReviewFeedback(
	ctx context.Context,
	specTask *types.SpecTask,
	review *types.SpecTaskDesignReview,
) error {
	// Get all unresolved comments
	comments, err := n.store.ListUnresolvedComments(ctx, review.ID)
	if err != nil {
		return fmt.Errorf("failed to list unresolved comments: %w", err)
	}

	// Build notification payload
	notification := map[string]interface{}{
		"type":          "design_review_changes_requested",
		"spec_task_id":  specTask.ID,
		"review_id":     review.ID,
		"review_status": review.Status,
		"overall_comment": review.OverallComment,
		"revision_count": specTask.SpecRevisionCount,
		"comments": n.formatCommentsForAgent(comments),
		"instructions": n.generateAgentInstructions(review, comments),
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("review_id", review.ID).
		Int("comment_count", len(comments)).
		Msg("[ReviewNotifier] Preparing to notify agent of review feedback")

	// TODO: Send via WebSocket to agent session
	// For now, just log the notification
	notificationJSON, _ := json.MarshalIndent(notification, "", "  ")
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("notification", string(notificationJSON)).
		Msg("[ReviewNotifier] Agent notification payload (WebSocket integration pending)")

	// TODO: Implement WebSocket push to agent
	// This would send to the agent's planning session:
	// - Get specTask.PlanningSessionID
	// - Find active WebSocket connection for that session
	// - Send notification via WebSocket

	return nil
}

// formatCommentsForAgent formats comments in a structured way for the agent
func (n *SpecTaskReviewNotifier) formatCommentsForAgent(comments []types.SpecTaskDesignReviewComment) []map[string]interface{} {
	formatted := make([]map[string]interface{}, len(comments))

	for i, comment := range comments {
		formatted[i] = map[string]interface{}{
			"id":            comment.ID,
			"document_type": comment.DocumentType,
			"section_path":  comment.SectionPath,
			"line_number":   comment.LineNumber,
			"quoted_text":   comment.QuotedText,
			"comment_text":  comment.CommentText,
			"comment_type":  comment.CommentType,
			"is_critical":   comment.CommentType == types.SpecTaskDesignReviewCommentTypeCritical,
		}
	}

	return formatted
}

// generateAgentInstructions generates natural language instructions for the agent
func (n *SpecTaskReviewNotifier) generateAgentInstructions(
	review *types.SpecTaskDesignReview,
	comments []types.SpecTaskDesignReviewComment,
) string {
	criticalCount := 0
	questionCount := 0
	suggestionCount := 0

	for _, comment := range comments {
		switch comment.CommentType {
		case types.SpecTaskDesignReviewCommentTypeCritical:
			criticalCount++
		case types.SpecTaskDesignReviewCommentTypeQuestion:
			questionCount++
		case types.SpecTaskDesignReviewCommentTypeSuggestion:
			suggestionCount++
		}
	}

	instructions := "The design review has been completed and changes have been requested.\n\n"

	if review.OverallComment != "" {
		instructions += fmt.Sprintf("**Overall Feedback:**\n%s\n\n", review.OverallComment)
	}

	instructions += "**Summary:**\n"
	if criticalCount > 0 {
		instructions += fmt.Sprintf("- %d critical issue(s) that must be addressed\n", criticalCount)
	}
	if questionCount > 0 {
		instructions += fmt.Sprintf("- %d question(s) requiring clarification\n", questionCount)
	}
	if suggestionCount > 0 {
		instructions += fmt.Sprintf("- %d suggestion(s) for improvement\n", suggestionCount)
	}

	instructions += "\n**Your Task:**\n"
	instructions += "1. Read all comments carefully, especially critical issues\n"
	instructions += "2. Update the design documents to address all feedback\n"
	instructions += "3. Make sure to:\n"
	instructions += "   - Answer all questions in the design docs\n"
	instructions += "   - Fix all critical issues\n"
	instructions += "   - Consider all suggestions\n"
	instructions += "4. Commit and push the updated design documents\n"
	instructions += "5. The updated design will automatically trigger a new review\n"

	return instructions
}

// NotifyAgentOfApproval notifies the agent that the design has been approved
func (n *SpecTaskReviewNotifier) NotifyAgentOfApproval(
	ctx context.Context,
	specTask *types.SpecTask,
	review *types.SpecTaskDesignReview,
) error {
	notification := map[string]interface{}{
		"type":          "design_review_approved",
		"spec_task_id":  specTask.ID,
		"review_id":     review.ID,
		"approved_at":   review.ApprovedAt,
		"approved_by":   specTask.SpecApprovedBy,
		"overall_comment": review.OverallComment,
		"instructions": "The design has been approved! You can now proceed to implementation phase.",
	}

	notificationJSON, _ := json.MarshalIndent(notification, "", "  ")
	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("notification", string(notificationJSON)).
		Msg("[ReviewNotifier] Design approved notification (WebSocket integration pending)")

	// TODO: Send via WebSocket to agent session
	return nil
}
