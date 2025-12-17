package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// AuditLogStore is a minimal interface for audit log operations
type AuditLogStore interface {
	CreateProjectAuditLog(ctx context.Context, log *types.ProjectAuditLog) error
	ListProjectAuditLogs(ctx context.Context, filters *types.ProjectAuditLogFilters) (*types.ProjectAuditLogResponse, error)
}

// AuditLogService handles audit log operations
type AuditLogService struct {
	store AuditLogStore
}

// NewAuditLogService creates a new audit log service
func NewAuditLogService(store AuditLogStore) *AuditLogService {
	return &AuditLogService{store: store}
}

// LogEvent creates a new audit log entry
// This method is fire-and-forget - errors are logged but not returned to avoid blocking main operations
func (s *AuditLogService) LogEvent(ctx context.Context, entry *types.ProjectAuditLog) {
	go s.logEventAsync(ctx, entry)
}

// logEventAsync performs the actual logging asynchronously
func (s *AuditLogService) logEventAsync(ctx context.Context, entry *types.ProjectAuditLog) {
	// Generate ID if not set
	if entry.ID == "" {
		entry.ID = uuid.New().String()
	}

	// Set timestamp if not set
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}

	// Create the log entry
	err := s.store.CreateProjectAuditLog(ctx, entry)
	if err != nil {
		log.Error().
			Err(err).
			Str("project_id", entry.ProjectID).
			Str("spec_task_id", entry.SpecTaskID).
			Str("event_type", string(entry.EventType)).
			Msg("Failed to create audit log entry")
	}
}

// LogTaskCreated logs a task creation event
func (s *AuditLogService) LogTaskCreated(ctx context.Context, task *types.SpecTask, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventTaskCreated,
		PromptText: task.OriginalPrompt,
		Metadata: types.AuditMetadata{
			TaskNumber: task.TaskNumber,
			TaskName:   task.Name,
			BranchName: task.BranchName,
		},
	})
}

// LogTaskCloned logs a task clone event
func (s *AuditLogService) LogTaskCloned(ctx context.Context, newTask *types.SpecTask, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  newTask.ProjectID,
		SpecTaskID: newTask.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventTaskCloned,
		PromptText: newTask.OriginalPrompt,
		Metadata: types.AuditMetadata{
			TaskNumber:          newTask.TaskNumber,
			TaskName:            newTask.Name,
			ClonedFromID:        newTask.ClonedFromID,
			ClonedFromProjectID: newTask.ClonedFromProjectID,
			CloneGroupID:        newTask.CloneGroupID,
		},
	})
}

// LogAgentPrompt logs a prompt sent from Helix UI to the agent
func (s *AuditLogService) LogAgentPrompt(ctx context.Context, task *types.SpecTask, prompt, sessionID, interactionID, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	metadata.SessionID = sessionID
	metadata.InteractionID = interactionID

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventAgentPrompt,
		PromptText: prompt,
		Metadata:   metadata,
	})
}

// LogUserMessage logs a message sent by user inside the agent (via WebSocket)
func (s *AuditLogService) LogUserMessage(ctx context.Context, projectID, specTaskID, message, sessionID, interactionID, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  projectID,
		SpecTaskID: specTaskID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventUserMessage,
		PromptText: message,
		Metadata: types.AuditMetadata{
			SessionID:     sessionID,
			InteractionID: interactionID,
		},
	})
}

// LogTaskApproved logs a task approval event
func (s *AuditLogService) LogTaskApproved(ctx context.Context, task *types.SpecTask, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	// Capture spec hashes at approval time
	metadata.RequirementsSpecHash = hashContent(task.RequirementsSpec)
	metadata.TechnicalDesignHash = hashContent(task.TechnicalDesign)
	metadata.ImplementationPlanHash = hashContent(task.ImplementationPlan)

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventTaskApproved,
		Metadata:   metadata,
	})
}

// LogPRCreated logs a pull request creation event
func (s *AuditLogService) LogPRCreated(ctx context.Context, task *types.SpecTask, prID, prURL, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	metadata.PullRequestID = prID
	metadata.PullRequestURL = prURL

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventPRCreated,
		Metadata:   metadata,
	})
}

// LogReviewComment logs a design review comment event
func (s *AuditLogService) LogReviewComment(ctx context.Context, task *types.SpecTask, reviewID, commentID, commentText, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	metadata.DesignReviewID = reviewID
	metadata.CommentID = commentID
	// Capture spec hashes at comment time
	metadata.RequirementsSpecHash = hashContent(task.RequirementsSpec)
	metadata.TechnicalDesignHash = hashContent(task.TechnicalDesign)
	metadata.ImplementationPlanHash = hashContent(task.ImplementationPlan)

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventReviewComment,
		PromptText: commentText,
		Metadata:   metadata,
	})
}

// LogSpecGenerated logs a spec generation event
func (s *AuditLogService) LogSpecGenerated(ctx context.Context, task *types.SpecTask, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	metadata.RequirementsSpecHash = hashContent(task.RequirementsSpec)
	metadata.TechnicalDesignHash = hashContent(task.TechnicalDesign)
	metadata.ImplementationPlanHash = hashContent(task.ImplementationPlan)

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventSpecGenerated,
		Metadata:   metadata,
	})
}

// LogAgentStarted logs an agent session start event
func (s *AuditLogService) LogAgentStarted(ctx context.Context, task *types.SpecTask, sessionID, userID, userEmail string) {
	metadata := s.buildTaskMetadata(task)
	metadata.SessionID = sessionID

	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventAgentStarted,
		Metadata:   metadata,
	})
}

// buildTaskMetadata creates common task metadata
func (s *AuditLogService) buildTaskMetadata(task *types.SpecTask) types.AuditMetadata {
	return types.AuditMetadata{
		TaskNumber:     task.TaskNumber,
		TaskName:       task.Name,
		BranchName:     task.BranchName,
		PullRequestID:  task.PullRequestID,
		PullRequestURL: task.PullRequestURL,
	}
}

// hashContent creates a SHA256 hash of content for versioning
func hashContent(content string) string {
	if content == "" {
		return ""
	}
	hash := sha256.Sum256([]byte(content))
	return hex.EncodeToString(hash[:8]) // First 8 bytes (16 hex chars) is enough for identification
}

// ListAuditLogs retrieves audit logs with filtering
func (s *AuditLogService) ListAuditLogs(ctx context.Context, filters *types.ProjectAuditLogFilters) (*types.ProjectAuditLogResponse, error) {
	return s.store.ListProjectAuditLogs(ctx, filters)
}
