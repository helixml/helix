package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
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
	store    AuditLogStore
	wg       *sync.WaitGroup // Optional WaitGroup for testing async operations
	testMode bool            // If true, skip async logging (used in tests)
}

// NewAuditLogService creates a new audit log service
func NewAuditLogService(store AuditLogStore) *AuditLogService {
	return &AuditLogService{store: store}
}

// SetWaitGroup sets a WaitGroup for tracking async operations (used in tests)
func (s *AuditLogService) SetWaitGroup(wg *sync.WaitGroup) {
	s.wg = wg
}

// SetTestMode enables or disables test mode (skips async logging)
func (s *AuditLogService) SetTestMode(enabled bool) {
	s.testMode = enabled
}

// LogEvent creates a new audit log entry
// This method is fire-and-forget - errors are logged but not returned to avoid blocking main operations
func (s *AuditLogService) LogEvent(ctx context.Context, entry *types.ProjectAuditLog) {
	// Skip async logging in test mode to avoid goroutine leaks
	if s.testMode {
		return
	}
	// Use background context since the HTTP request context may be canceled before the goroutine runs
	if s.wg != nil {
		s.wg.Add(1)
	}
	go func() {
		if s.wg != nil {
			defer s.wg.Done()
		}
		s.logEventAsync(context.Background(), entry)
	}()
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

// LogTaskArchived logs a task archive event
func (s *AuditLogService) LogTaskArchived(ctx context.Context, task *types.SpecTask, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventTaskArchived,
		PromptText: "Archived task: " + task.Name,
		Metadata: types.AuditMetadata{
			TaskNumber: task.TaskNumber,
			TaskName:   task.Name,
			BranchName: task.BranchName,
		},
	})
}

// LogTaskUnarchived logs a task unarchive event
func (s *AuditLogService) LogTaskUnarchived(ctx context.Context, task *types.SpecTask, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  task.ProjectID,
		SpecTaskID: task.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventTaskUnarchived,
		PromptText: "Unarchived task: " + task.Name,
		Metadata: types.AuditMetadata{
			TaskNumber: task.TaskNumber,
			TaskName:   task.Name,
			BranchName: task.BranchName,
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

// LogProjectCreated logs a project creation event
func (s *AuditLogService) LogProjectCreated(ctx context.Context, project *types.Project, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  project.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventProjectCreated,
		PromptText: "Created project: " + project.Name,
		Metadata: types.AuditMetadata{
			ProjectName: project.Name,
		},
	})
}

// LogProjectDeleted logs a project deletion event
func (s *AuditLogService) LogProjectDeleted(ctx context.Context, project *types.Project, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  project.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventProjectDeleted,
		PromptText: "Deleted project: " + project.Name,
		Metadata: types.AuditMetadata{
			ProjectName: project.Name,
		},
	})
}

// LogProjectSettingsUpdated logs a project settings update event
func (s *AuditLogService) LogProjectSettingsUpdated(ctx context.Context, project *types.Project, changedFields []string, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID:  project.ID,
		UserID:     userID,
		UserEmail:  userEmail,
		EventType:  types.AuditEventProjectSettingsUpdated,
		PromptText: "Updated settings: " + joinStrings(changedFields),
		Metadata: types.AuditMetadata{
			ProjectName: project.Name,
		},
	})
}

// LogProjectGuidelinesUpdated logs a project guidelines update event
func (s *AuditLogService) LogProjectGuidelinesUpdated(ctx context.Context, project *types.Project, userID, userEmail string) {
	s.LogEvent(ctx, &types.ProjectAuditLog{
		ProjectID: project.ID,
		UserID:    userID,
		UserEmail: userEmail,
		EventType: types.AuditEventProjectGuidelinesUpdated,
		Metadata: types.AuditMetadata{
			ProjectName: project.Name,
		},
	})
}

// joinStrings joins strings with comma separator
func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += ", " + strs[i]
	}
	return result
}

// ListAuditLogs retrieves audit logs with filtering
func (s *AuditLogService) ListAuditLogs(ctx context.Context, filters *types.ProjectAuditLogFilters) (*types.ProjectAuditLogResponse, error) {
	return s.store.ListProjectAuditLogs(ctx, filters)
}
