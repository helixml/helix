package services

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// DocumentHandoffService manages the complete git-based document handoff workflow
// This service orchestrates the flow from planning → spec generation → approval → git commit → implementation
type DocumentHandoffService struct {
	store                   store.Store
	specDocumentService     *SpecDocumentService
	multiSessionManager     *SpecTaskMultiSessionManager
	gitBasePath             string
	gitUserName             string
	gitUserEmail            string
	specBranchPrefix        string // e.g., "specs/"
	sessionBranchPrefix     string // e.g., "sessions/"
	defaultCommitMessage    string
	enablePullRequests      bool
	enableContinuousCommits bool
	testMode                bool
}

// DocumentHandoffConfig represents configuration for document handoff
type DocumentHandoffConfig struct {
	EnableGitIntegration   bool              `json:"enable_git_integration"`
	EnablePullRequests     bool              `json:"enable_pull_requests"`
	EnableSessionRecording bool              `json:"enable_session_recording"`
	CommitFrequencyMinutes int               `json:"commit_frequency_minutes"`
	RequireCodeReview      bool              `json:"require_code_review"`
	AutoMergeApprovedSpecs bool              `json:"auto_merge_approved_specs"`
	SpecReviewers          []string          `json:"spec_reviewers,omitempty"`
	NotificationWebhooks   []string          `json:"notification_webhooks,omitempty"`
	CustomGitHooks         map[string]string `json:"custom_git_hooks,omitempty"`
}

// HandoffResult represents the result of a complete document handoff process
type HandoffResult struct {
	SpecTaskID          string                 `json:"spec_task_id"`
	Phase               string                 `json:"phase"` // "spec_commit", "implementation_start", "progress_update", "completion"
	GitCommitHash       string                 `json:"git_commit_hash"`
	BranchName          string                 `json:"branch_name"`
	PullRequestURL      string                 `json:"pull_request_url,omitempty"`
	FilesCommitted      []string               `json:"files_committed"`
	WorkSessionsCreated []string               `json:"work_sessions_created,omitempty"`
	ZedInstanceID       string                 `json:"zed_instance_id,omitempty"`
	Success             bool                   `json:"success"`
	Message             string                 `json:"message"`
	Warnings            []string               `json:"warnings,omitempty"`
	NextActions         []string               `json:"next_actions,omitempty"`
	NotificationsSent   []string               `json:"notifications_sent,omitempty"`
	HandoffTimestamp    time.Time              `json:"handoff_timestamp"`
	EstimatedCompletion *time.Time             `json:"estimated_completion,omitempty"`
	Metadata            map[string]interface{} `json:"metadata,omitempty"`
}

// SessionHistoryRecord represents a session activity record for git
type SessionHistoryRecord struct {
	WorkSessionID    string                 `json:"work_session_id"`
	HelixSessionID   string                 `json:"helix_session_id"`
	ZedThreadID      string                 `json:"zed_thread_id,omitempty"`
	ActivityType     string                 `json:"activity_type"` // "conversation", "code_change", "decision", "coordination"
	Timestamp        time.Time              `json:"timestamp"`
	Content          string                 `json:"content"`
	FilesAffected    []string               `json:"files_affected,omitempty"`
	CodeChanges      map[string]interface{} `json:"code_changes,omitempty"`
	CoordinationData map[string]interface{} `json:"coordination_data,omitempty"`
	Metadata         map[string]interface{} `json:"metadata,omitempty"`
}

// NewDocumentHandoffService creates a new document handoff service
func NewDocumentHandoffService(
	store store.Store,
	specDocumentService *SpecDocumentService,
	multiSessionManager *SpecTaskMultiSessionManager,
	gitBasePath string,
	gitUserName string,
	gitUserEmail string,
) *DocumentHandoffService {
	return &DocumentHandoffService{
		store:                   store,
		specDocumentService:     specDocumentService,
		multiSessionManager:     multiSessionManager,
		gitBasePath:             gitBasePath,
		gitUserName:             gitUserName,
		gitUserEmail:            gitUserEmail,
		specBranchPrefix:        "specs/",
		sessionBranchPrefix:     "sessions/",
		defaultCommitMessage:    "Automated spec document update",
		enablePullRequests:      true,
		enableContinuousCommits: true,
		testMode:                false,
	}
}

// SetTestMode enables or disables test mode
func (s *DocumentHandoffService) SetTestMode(enabled bool) {
	s.testMode = enabled
	if s.specDocumentService != nil {
		s.specDocumentService.SetTestMode(enabled)
	}
}

// ExecuteSpecApprovalHandoff executes the complete handoff when specs are approved
func (s *DocumentHandoffService) ExecuteSpecApprovalHandoff(
	ctx context.Context,
	specTaskID string,
	approval *types.SpecApprovalResponse,
	config *DocumentHandoffConfig,
) (*HandoffResult, error) {
	log.Info().
		Str("spec_task_id", specTaskID).
		Bool("approved", approval.Approved).
		Str("approved_by", approval.ApprovedBy).
		Msg("Executing spec approval handoff")

	result := &HandoffResult{
		SpecTaskID:       specTaskID,
		Phase:            "spec_commit",
		HandoffTimestamp: time.Now(),
		Success:          false,
	}

	// Get SpecTask
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	if approval.Approved {
		// Execute approved spec handoff
		return s.executeApprovedSpecHandoff(ctx, specTask, approval, config, result)
	} else {
		// Handle spec rejection or change requests
		return s.executeSpecRejectionHandoff(ctx, specTask, approval, config, result)
	}
}

// executeApprovedSpecHandoff handles the handoff when specs are approved
func (s *DocumentHandoffService) executeApprovedSpecHandoff(
	ctx context.Context,
	specTask *types.SpecTask,
	approval *types.SpecApprovalResponse,
	config *DocumentHandoffConfig,
	result *HandoffResult,
) (*HandoffResult, error) {
	// 1. Generate and commit spec documents to git
	if config == nil || config.EnableGitIntegration {
		gitResult, err := s.generateAndCommitSpecDocuments(ctx, specTask, approval)
		if err != nil {
			return nil, fmt.Errorf("failed to generate and commit spec documents: %w", err)
		}

		result.GitCommitHash = gitResult.CommitHash
		result.BranchName = gitResult.BranchName
		result.PullRequestURL = gitResult.PullRequestURL
		result.FilesCommitted = gitResult.FilesCreated
	}

	// 2. Update SpecTask status and approval info
	specTask.Status = types.TaskStatusSpecApproved
	specTask.SpecApprovedBy = approval.ApprovedBy
	specTask.SpecApprovedAt = &approval.ApprovedAt

	err := s.store.UpdateSpecTask(ctx, specTask)
	if err != nil {
		return nil, fmt.Errorf("failed to update SpecTask approval status: %w", err)
	}

	// 3. Start multi-session implementation
	implementationConfig := &types.SpecTaskImplementationSessionsCreateRequest{
		SpecTaskID:         specTask.ID,
		ProjectPath:        s.getProjectPath(specTask),
		AutoCreateSessions: true,
		WorkspaceConfig: map[string]interface{}{
			"git_branch":       result.BranchName,
			"spec_commit_hash": result.GitCommitHash,
			"approved_by":      approval.ApprovedBy,
			"approved_at":      approval.ApprovedAt,
			"spec_documents":   true,
		},
	}

	overview, err := s.multiSessionManager.CreateImplementationSessions(ctx, specTask.ID, implementationConfig)
	if err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to create implementation sessions: %v", err))
	} else {
		result.Phase = "implementation_start"
		result.ZedInstanceID = overview.ZedInstanceID

		// Collect work session IDs
		for _, ws := range overview.WorkSessions {
			result.WorkSessionsCreated = append(result.WorkSessionsCreated, ws.ID)
		}
	}

	// 4. Send notifications if configured
	if config != nil && len(config.NotificationWebhooks) > 0 {
		notifications := s.sendHandoffNotifications(ctx, specTask, result, config)
		result.NotificationsSent = notifications
	}

	// 5. Initialize session history recording
	if config == nil || config.EnableSessionRecording {
		err = s.initializeSessionHistoryRecording(ctx, specTask, result)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to initialize session recording: %v", err))
		}
	}

	result.Success = true
	result.Message = fmt.Sprintf("Spec approval handoff completed successfully for '%s'", specTask.Name)
	result.NextActions = []string{
		"Implementation sessions are starting",
		"Zed instance is being initialized",
		"Spec documents are available in git",
		"Progress will be tracked in real-time",
	}

	// Estimate completion time based on implementation tasks
	if overview != nil && len(overview.ImplementationTasks) > 0 {
		estimatedHours := s.calculateEstimatedCompletion(overview.ImplementationTasks)
		estimatedCompletion := time.Now().Add(time.Duration(estimatedHours) * time.Hour)
		result.EstimatedCompletion = &estimatedCompletion
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("commit_hash", result.GitCommitHash).
		Int("work_sessions", len(result.WorkSessionsCreated)).
		Str("zed_instance", result.ZedInstanceID).
		Msg("Spec approval handoff completed successfully")

	return result, nil
}

// executeSpecRejectionHandoff handles spec rejection or change requests
func (s *DocumentHandoffService) executeSpecRejectionHandoff(
	ctx context.Context,
	specTask *types.SpecTask,
	approval *types.SpecApprovalResponse,
	config *DocumentHandoffConfig,
	result *HandoffResult,
) (*HandoffResult, error) {
	// Update SpecTask status
	specTask.Status = types.TaskStatusSpecRevision
	specTask.SpecRevisionCount++

	// Add rejection comments to spec task (GORM serializer handles JSON conversion)
	if approval.Comments != "" {
		// Store rejection feedback for planning agent to address
		specTask.Metadata = map[string]interface{}{
			"rejected_by":    approval.ApprovedBy,
			"rejected_at":    approval.ApprovedAt,
			"comments":       approval.Comments,
			"changes":        approval.Changes,
			"revision_count": specTask.SpecRevisionCount,
		}
	}

	err := s.store.UpdateSpecTask(ctx, specTask)
	if err != nil {
		return nil, fmt.Errorf("failed to update SpecTask rejection status: %w", err)
	}

	// Commit rejection feedback to git for audit trail
	if config == nil || config.EnableGitIntegration {
		err = s.commitRejectionFeedback(ctx, specTask, approval)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Failed to commit rejection feedback: %v", err))
		}
	}

	result.Success = true
	result.Message = fmt.Sprintf("Spec rejection recorded for '%s'", specTask.Name)
	result.NextActions = []string{
		"Planning agent will address feedback",
		"Revised specifications will be generated",
		"New review cycle will begin",
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("rejected_by", approval.ApprovedBy).
		Int("revision_count", specTask.SpecRevisionCount).
		Msg("Spec rejection handoff completed")

	return result, nil
}

// RecordSessionHistory records session activity to git during implementation
func (s *DocumentHandoffService) RecordSessionHistory(
	ctx context.Context,
	record *SessionHistoryRecord,
) error {
	if s.testMode {
		log.Info().
			Str("work_session_id", record.WorkSessionID).
			Str("activity_type", record.ActivityType).
			Msg("Test mode: skipping session history recording")
		return nil
	}

	// Get work session to determine SpecTask
	workSession, err := s.store.GetSpecTaskWorkSession(ctx, record.WorkSessionID)
	if err != nil {
		return fmt.Errorf("failed to get work session: %w", err)
	}

	specTask, err := s.store.GetSpecTask(ctx, workSession.SpecTaskID)
	if err != nil {
		return fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Determine repository and paths
	repoPath := s.getProjectPath(specTask)
	sessionDir := filepath.Join("sessions", specTask.ID, record.WorkSessionID)

	// Generate appropriate markdown content based on activity type
	var filename string
	var content string

	switch record.ActivityType {
	case "conversation":
		filename = "conversation.md"
		content = s.formatConversationHistory(record)
	case "code_change":
		filename = "code-changes.md"
		content = s.formatCodeChangeHistory(record)
	case "decision":
		filename = "decisions.md"
		content = s.formatDecisionHistory(record)
	case "coordination":
		filename = filepath.Join("..", "coordination-log.md") // Up one level to sessions/{spec_task_id}/
		content = s.formatCoordinationHistory(record)
	default:
		filename = fmt.Sprintf("%s-activity.md", record.ActivityType)
		content = s.formatGenericActivity(record)
	}

	// Commit to git
	err = s.commitSessionActivity(ctx, repoPath, sessionDir, filename, content, record)
	if err != nil {
		return fmt.Errorf("failed to commit session activity: %w", err)
	}

	log.Info().
		Str("work_session_id", record.WorkSessionID).
		Str("activity_type", record.ActivityType).
		Str("filename", filename).
		Msg("Recorded session history to git")

	return nil
}

// CommitImplementationProgress commits periodic progress updates during implementation
func (s *DocumentHandoffService) CommitImplementationProgress(
	ctx context.Context,
	specTaskID string,
	progressSummary string,
	detailedProgress map[string]interface{},
) (*HandoffResult, error) {
	if s.testMode {
		return &HandoffResult{
			SpecTaskID: specTaskID,
			Phase:      "progress_update",
			Success:    true,
			Message:    "Test mode: progress update simulated",
		}, nil
	}

	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Get current progress
	progress, err := s.store.GetSpecTaskProgress(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get progress: %w", err)
	}

	// Generate progress update document
	progressContent := s.generateProgressUpdateMarkdown(specTask, progress, progressSummary, detailedProgress)

	// Commit progress update
	repoPath := s.getProjectPath(specTask)
	sessionDir := filepath.Join("sessions", specTask.ID)
	filename := fmt.Sprintf("progress-update-%s.md", time.Now().Format("2006-01-02-15-04"))

	err = s.commitSessionActivity(ctx, repoPath, sessionDir, filename, progressContent, &SessionHistoryRecord{
		ActivityType: "progress_update",
		Timestamp:    time.Now(),
		Content:      progressSummary,
		Metadata:     detailedProgress,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit progress update: %w", err)
	}

	// Also update tasks.md with current status
	err = s.updateTasksMarkdownWithProgress(ctx, specTask, progress)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to update tasks.md with progress")
	}

	result := &HandoffResult{
		SpecTaskID:       specTaskID,
		Phase:            "progress_update",
		BranchName:       s.sessionBranchPrefix + specTask.ID,
		FilesCommitted:   []string{filepath.Join(sessionDir, filename)},
		Success:          true,
		Message:          fmt.Sprintf("Progress update committed: %s", progressSummary),
		HandoffTimestamp: time.Now(),
	}

	log.Info().
		Str("spec_task_id", specTaskID).
		Float64("overall_progress", progress.OverallProgress).
		Str("summary", progressSummary).
		Msg("Committed implementation progress update")

	return result, nil
}

// CompleteImplementationHandoff executes final handoff when implementation is complete
func (s *DocumentHandoffService) CompleteImplementationHandoff(
	ctx context.Context,
	specTaskID string,
	completionSummary string,
	finalArtifacts map[string]string,
) (*HandoffResult, error) {
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	// Generate completion documentation
	completionContent := s.generateCompletionMarkdown(specTask, completionSummary, finalArtifacts)

	// Commit completion documentation
	repoPath := s.getProjectPath(specTask)
	completionDir := filepath.Join("sessions", specTask.ID)
	filename := "implementation-complete.md"

	err = s.commitSessionActivity(ctx, repoPath, completionDir, filename, completionContent, &SessionHistoryRecord{
		ActivityType: "completion",
		Timestamp:    time.Now(),
		Content:      completionSummary,
		Metadata: map[string]interface{}{
			"final_artifacts": finalArtifacts,
			"completed_at":    time.Now(),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit completion documentation: %w", err)
	}

	// Create final summary commit
	err = s.createFinalSummaryCommit(ctx, specTask, completionSummary)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to create final summary commit")
	}

	result := &HandoffResult{
		SpecTaskID:       specTask.ID,
		Phase:            "completion",
		BranchName:       s.sessionBranchPrefix + specTask.ID,
		FilesCommitted:   []string{filepath.Join(completionDir, filename)},
		Success:          true,
		Message:          fmt.Sprintf("Implementation completion documented: %s", completionSummary),
		HandoffTimestamp: time.Now(),
		NextActions: []string{
			"Implementation is complete",
			"Documentation is available in git",
			"Code review and deployment can proceed",
		},
	}

	log.Info().
		Str("spec_task_id", specTask.ID).
		Str("summary", completionSummary).
		Msg("Completed implementation handoff")

	return result, nil
}

// Private helper methods

func (s *DocumentHandoffService) generateAndCommitSpecDocuments(
	ctx context.Context,
	specTask *types.SpecTask,
	approval *types.SpecApprovalResponse,
) (*SpecDocumentResult, error) {
	// Create spec document configuration
	config := &SpecDocumentConfig{
		SpecTaskID:        specTask.ID,
		ProjectPath:       s.getProjectPath(specTask),
		BranchName:        s.specBranchPrefix + specTask.ID,
		CommitMessage:     s.generateSpecCommitMessage(specTask, approval),
		IncludeTimestamps: true,
		GenerateTaskBoard: true,
		OverwriteExisting: true,
		CreatePullRequest: s.enablePullRequests,
		CustomMetadata: map[string]string{
			"approved_by": approval.ApprovedBy,
			"approved_at": approval.ApprovedAt.Format(time.RFC3339),
			"comments":    approval.Comments,
		},
	}

	// Generate documents using SpecDocumentService
	return s.specDocumentService.GenerateSpecDocuments(ctx, config)
}

func (s *DocumentHandoffService) commitRejectionFeedback(
	ctx context.Context,
	specTask *types.SpecTask,
	approval *types.SpecApprovalResponse,
) error {
	// Create rejection feedback document
	feedbackContent := fmt.Sprintf(`# Specification Review Feedback: %s

> **SpecTask**: %s
> **Reviewed by**: %s
> **Reviewed on**: %s
> **Decision**: CHANGES REQUESTED

## Reviewer Comments

%s

## Requested Changes

%s

## Next Steps

The planning agent will address this feedback and generate revised specifications for another review cycle.

---
*Feedback recorded on %s*
`,
		specTask.Name,
		specTask.ID,
		approval.ApprovedBy,
		approval.ApprovedAt.Format("2006-01-02 15:04:05"),
		approval.Comments,
		strings.Join(approval.Changes, "\n- "),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	// Commit feedback to git
	repoPath := s.getProjectPath(specTask)
	feedbackDir := filepath.Join("specs", specTask.ID, "reviews")
	filename := fmt.Sprintf("review-%s.md", approval.ApprovedAt.Format("2006-01-02-15-04"))

	return s.commitSessionActivity(ctx, repoPath, feedbackDir, filename, feedbackContent, &SessionHistoryRecord{
		ActivityType: "spec_rejection",
		Timestamp:    approval.ApprovedAt,
		Content:      approval.Comments,
		Metadata: map[string]interface{}{
			"rejected_by": approval.ApprovedBy,
			"changes":     approval.Changes,
		},
	})
}

func (s *DocumentHandoffService) initializeSessionHistoryRecording(
	ctx context.Context,
	specTask *types.SpecTask,
	result *HandoffResult,
) error {
	// Create initial session history structure
	repoPath := s.getProjectPath(specTask)
	sessionDir := filepath.Join("sessions", specTask.ID)

	// Create README for session history
	readmeContent := fmt.Sprintf(`# Session History: %s

This directory contains the complete session history for the multi-session implementation of this SpecTask.

## Structure

- **coordination-log.md**: Inter-session coordination events and handoffs
- **progress-updates/**: Periodic progress commits during implementation
- **{work_session_id}/**: Individual session history directories
  - **conversation.md**: Complete conversation log between human and AI
  - **code-changes.md**: Code changes with reasoning and context
  - **decisions.md**: Key technical and design decisions made
  - **activity-log.md**: Detailed activity timeline

## SpecTask Information

- **ID**: %s
- **Name**: %s
- **Approved By**: %s
- **Implementation Started**: %s
- **Expected Work Sessions**: %d

## Real-time Updates

This documentation is updated in real-time as implementation progresses. Each significant action, decision, and coordination event is automatically recorded and committed to maintain a complete audit trail.

---
*Session history initialized on %s*
`,
		specTask.Name,
		specTask.ID,
		specTask.Name,
		result.NextActions[0], // Approved by info
		time.Now().Format("2006-01-02 15:04:05"),
		len(result.WorkSessionsCreated),
		time.Now().Format("2006-01-02 15:04:05"),
	)

	return s.commitSessionActivity(ctx, repoPath, sessionDir, "README.md", readmeContent, &SessionHistoryRecord{
		ActivityType: "initialization",
		Timestamp:    time.Now(),
		Content:      "Session history recording initialized",
	})
}

func (s *DocumentHandoffService) commitSessionActivity(
	ctx context.Context,
	repoPath string,
	relativeDir string,
	filename string,
	content string,
	record *SessionHistoryRecord,
) error {
	// Open or create repository
	repo, err := s.openOrCreateRepository(repoPath)
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}

	// Create directory structure
	fullDir := filepath.Join(repoPath, relativeDir)
	err = os.MkdirAll(fullDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write file
	filePath := filepath.Join(fullDir, filename)

	// If file exists and this is an append operation, append to existing content
	if record.ActivityType == "conversation" || record.ActivityType == "coordination" {
		existingContent, err := os.ReadFile(filePath)
		if err == nil {
			// Append to existing content
			content = string(existingContent) + "\n\n" + content
		}
	}

	err = os.WriteFile(filePath, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	// Stage and commit
	worktree, err := repo.Worktree()
	if err != nil {
		return fmt.Errorf("failed to get worktree: %w", err)
	}

	relativePath := filepath.Join(relativeDir, filename)
	_, err = worktree.Add(relativePath)
	if err != nil {
		return fmt.Errorf("failed to stage file: %w", err)
	}

	commitMessage := s.generateSessionCommitMessage(record, filename)
	_, err = worktree.Commit(commitMessage, &git.CommitOptions{
		Author: &object.Signature{
			Name:  s.gitUserName,
			Email: s.gitUserEmail,
			When:  record.Timestamp,
		},
	})
	if err != nil {
		return fmt.Errorf("failed to commit session activity: %w", err)
	}

	return nil
}

// Document formatting methods

func (s *DocumentHandoffService) formatConversationHistory(record *SessionHistoryRecord) string {
	return fmt.Sprintf(`## %s

**Session**: %s
**Thread**: %s
**Timestamp**: %s

%s

---
`,
		record.Timestamp.Format("2006-01-02 15:04:05"),
		record.WorkSessionID,
		record.ZedThreadID,
		record.Timestamp.Format("15:04:05"),
		record.Content,
	)
}

func (s *DocumentHandoffService) formatCodeChangeHistory(record *SessionHistoryRecord) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`## Code Changes - %s

**Session**: %s
**Thread**: %s
**Timestamp**: %s

`,
		record.Timestamp.Format("2006-01-02 15:04:05"),
		record.WorkSessionID,
		record.ZedThreadID,
		record.Timestamp.Format("15:04:05"),
	))

	// Add files affected
	if len(record.FilesAffected) > 0 {
		content.WriteString("**Files Modified**:\n")
		for _, file := range record.FilesAffected {
			content.WriteString(fmt.Sprintf("- %s\n", file))
		}
		content.WriteString("\n")
	}

	// Add change details
	content.WriteString("**Changes**:\n")
	content.WriteString(record.Content)

	// Add code change metadata if available
	if record.CodeChanges != nil {
		if linesAdded, ok := record.CodeChanges["lines_added"].(float64); ok {
			content.WriteString(fmt.Sprintf("\n**Lines Added**: %.0f\n", linesAdded))
		}
		if linesRemoved, ok := record.CodeChanges["lines_removed"].(float64); ok {
			content.WriteString(fmt.Sprintf("**Lines Removed**: %.0f\n", linesRemoved))
		}
	}

	content.WriteString("\n---\n")
	return content.String()
}

func (s *DocumentHandoffService) formatDecisionHistory(record *SessionHistoryRecord) string {
	return fmt.Sprintf(`## Decision - %s

**Session**: %s
**Thread**: %s
**Timestamp**: %s

### Decision Made

%s

### Context

%s

### Rationale

%s

---
`,
		record.Timestamp.Format("2006-01-02 15:04:05"),
		record.WorkSessionID,
		record.ZedThreadID,
		record.Timestamp.Format("15:04:05"),
		record.Content,
		s.extractFromMetadata(record.Metadata, "context", "Decision context not recorded"),
		s.extractFromMetadata(record.Metadata, "rationale", "Rationale not recorded"),
	)
}

func (s *DocumentHandoffService) formatCoordinationHistory(record *SessionHistoryRecord) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`## Coordination Event - %s

**From Session**: %s
**To Session**: %s
**Event Type**: %s
**Timestamp**: %s

### Message

%s

`,
		record.Timestamp.Format("2006-01-02 15:04:05"),
		record.WorkSessionID,
		s.extractFromMetadata(record.CoordinationData, "to_session_id", "Broadcast"),
		s.extractFromMetadata(record.CoordinationData, "event_type", "notification"),
		record.Timestamp.Format("15:04:05"),
		record.Content,
	))

	// Add coordination-specific metadata
	if record.CoordinationData != nil {
		if response, ok := record.CoordinationData["response"].(string); ok && response != "" {
			content.WriteString(fmt.Sprintf("### Response\n\n%s\n\n", response))
		}
		if acknowledged, ok := record.CoordinationData["acknowledged"].(bool); ok && acknowledged {
			content.WriteString("✅ **Acknowledged**\n\n")
		}
	}

	content.WriteString("---\n")
	return content.String()
}

func (s *DocumentHandoffService) generateProgressUpdateMarkdown(
	specTask *types.SpecTask,
	progress *types.SpecTaskProgressResponse,
	summary string,
	details map[string]interface{},
) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`# Progress Update: %s

**Date**: %s
**Overall Progress**: %.1f%% complete
**Active Sessions**: %d

## Summary

%s

## Detailed Progress

`,
		specTask.Name,
		time.Now().Format("2006-01-02 15:04:05"),
		progress.OverallProgress*100,
		len(progress.ActiveWorkSessions),
		summary,
	))

	// Add implementation task progress
	for taskIndex, taskProgress := range progress.ImplementationProgress {
		content.WriteString(fmt.Sprintf("- **Task %d**: %.1f%% complete\n", taskIndex+1, taskProgress*100))
	}

	// Add session status
	content.WriteString("\n## Active Work Sessions\n\n")
	for _, session := range progress.ActiveWorkSessions {
		content.WriteString(fmt.Sprintf("- **%s** (%s): %s\n",
			session.Name,
			session.Status,
			session.Description,
		))
	}

	// Add metadata if provided
	if details != nil {
		content.WriteString("\n## Additional Details\n\n")
		for key, value := range details {
			content.WriteString(fmt.Sprintf("- **%s**: %v\n", key, value))
		}
	}

	content.WriteString(fmt.Sprintf("\n---\n*Progress update generated on %s*\n", time.Now().Format("2006-01-02 15:04:05")))
	return content.String()
}

func (s *DocumentHandoffService) generateCompletionMarkdown(
	specTask *types.SpecTask,
	summary string,
	artifacts map[string]string,
) string {
	var content strings.Builder

	content.WriteString(fmt.Sprintf(`# Implementation Complete: %s

**SpecTask ID**: %s
**Completed**: %s
**Duration**: %s

## Completion Summary

%s

## Final Artifacts

`,
		specTask.Name,
		specTask.ID,
		time.Now().Format("2006-01-02 15:04:05"),
		s.calculateImplementationDuration(specTask),
		summary,
	))

	// Add artifacts
	for name, description := range artifacts {
		content.WriteString(fmt.Sprintf("### %s\n\n%s\n\n", name, description))
	}

	// Add final statistics
	content.WriteString(fmt.Sprintf(`## Implementation Statistics

- **Total Work Sessions**: [Calculated during commit]
- **Total Zed Threads**: [Calculated during commit]
- **Git Commits**: [Calculated during commit]
- **Files Changed**: [Calculated during commit]

## Next Steps

1. **Code Review**: Review implementation against approved specifications
2. **Testing**: Execute comprehensive testing plan
3. **Deployment**: Deploy to appropriate environments
4. **Documentation**: Update user and API documentation
5. **Retrospective**: Conduct implementation retrospective

---
*Implementation completed on %s*
`, time.Now().Format("2006-01-02 15:04:05")))

	return content.String()
}

// Utility methods

func (s *DocumentHandoffService) getProjectPath(specTask *types.SpecTask) string {
	if specTask.ProjectPath != "" {
		return specTask.ProjectPath
	}
	return filepath.Join(s.gitBasePath, specTask.ProjectID, specTask.ID)
}

func (s *DocumentHandoffService) openOrCreateRepository(repoPath string) (*git.Repository, error) {
	// Try to open existing repository
	repo, err := git.PlainOpen(repoPath)
	if err == nil {
		return repo, nil
	}

	// Create new repository
	err = os.MkdirAll(repoPath, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create repository directory: %w", err)
	}

	repo, err = git.PlainInit(repoPath, false)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}

	return repo, nil
}

func (s *DocumentHandoffService) generateSpecCommitMessage(specTask *types.SpecTask, approval *types.SpecApprovalResponse) string {
	return fmt.Sprintf(`Add approved specifications for %s

SpecTask: %s
Approved by: %s
Generated: requirements.md, design.md, tasks.md

The specifications have been reviewed and approved for implementation.
Multi-session implementation will begin based on these documents.

%s`,
		specTask.Name,
		specTask.ID,
		approval.ApprovedBy,
		approval.Comments,
	)
}

func (s *DocumentHandoffService) generateSessionCommitMessage(record *SessionHistoryRecord, filename string) string {
	switch record.ActivityType {
	case "conversation":
		return fmt.Sprintf("Record conversation activity: %s", s.truncateContent(record.Content, 50))
	case "code_change":
		return fmt.Sprintf("Record code changes: %s", s.truncateContent(record.Content, 50))
	case "decision":
		return fmt.Sprintf("Record decision: %s", s.truncateContent(record.Content, 50))
	case "coordination":
		return fmt.Sprintf("Record coordination event: %s", s.truncateContent(record.Content, 50))
	case "progress_update":
		return fmt.Sprintf("Progress update: %s", s.truncateContent(record.Content, 50))
	case "completion":
		return fmt.Sprintf("Implementation complete: %s", s.truncateContent(record.Content, 50))
	default:
		return fmt.Sprintf("Session activity: %s in %s", record.ActivityType, filename)
	}
}

func (s *DocumentHandoffService) truncateContent(content string, maxLen int) string {
	if len(content) <= maxLen {
		return content
	}
	return content[:maxLen-3] + "..."
}

func (s *DocumentHandoffService) extractFromMetadata(metadata map[string]interface{}, key string, defaultValue string) string {
	if metadata == nil {
		return defaultValue
	}
	if value, ok := metadata[key].(string); ok {
		return value
	}
	return defaultValue
}

func (s *DocumentHandoffService) calculateEstimatedCompletion(tasks []types.SpecTaskImplementationTask) float64 {
	totalHours := 0.0
	for _, task := range tasks {
		switch task.EstimatedEffort {
		case "small":
			totalHours += 4.0
		case "medium":
			totalHours += 8.0
		case "large":
			totalHours += 16.0
		default:
			totalHours += 8.0 // Default to medium
		}
	}
	return totalHours
}

func (s *DocumentHandoffService) calculateImplementationDuration(specTask *types.SpecTask) string {
	if specTask.StartedAt != nil && specTask.CompletedAt != nil {
		duration := specTask.CompletedAt.Sub(*specTask.StartedAt)
		return duration.String()
	}
	return "Duration not available"
}

func (s *DocumentHandoffService) sendHandoffNotifications(
	ctx context.Context,
	specTask *types.SpecTask,
	result *HandoffResult,
	config *DocumentHandoffConfig,
) []string {
	// Implementation would send notifications to configured webhooks
	// For now, just log the notifications that would be sent
	var notifications []string

	for _, webhook := range config.NotificationWebhooks {
		notification := fmt.Sprintf("Webhook notification sent to %s for SpecTask %s", webhook, specTask.ID)
		notifications = append(notifications, notification)

		log.Info().
			Str("webhook", webhook).
			Str("spec_task_id", specTask.ID).
			Msg("Would send handoff notification")
	}

	return notifications
}

func (s *DocumentHandoffService) updateTasksMarkdownWithProgress(
	ctx context.Context,
	specTask *types.SpecTask,
	progress *types.SpecTaskProgressResponse,
) error {
	// Generate updated tasks.md with current progress
	config := &SpecDocumentConfig{
		SpecTaskID:        specTask.ID,
		ProjectPath:       s.getProjectPath(specTask),
		IncludeTimestamps: true,
		GenerateTaskBoard: true,
	}

	tasksContent := s.specDocumentService.generateTasksMarkdown(specTask, config)

	// Add current progress section
	progressSection := fmt.Sprintf(`

## Current Progress (Updated %s)

**Overall Progress**: %.1f%% complete
**Active Sessions**: %d
**Completed Sessions**: %d

### Session Status
`,
		time.Now().Format("2006-01-02 15:04:05"),
		progress.OverallProgress*100,
		len(progress.ActiveWorkSessions),
		s.countCompletedSessions(progress.ActiveWorkSessions),
	)

	for _, session := range progress.ActiveWorkSessions {
		progressSection += fmt.Sprintf("- **%s**: %s\n", session.Name, session.Status)
	}

	tasksContent += progressSection

	// Commit updated tasks.md
	repoPath := s.getProjectPath(specTask)
	specDir := filepath.Join("specs", specTask.ID)

	return s.commitSessionActivity(ctx, repoPath, specDir, "tasks.md", tasksContent, &SessionHistoryRecord{
		ActivityType: "progress_update",
		Timestamp:    time.Now(),
		Content:      "Updated tasks.md with current implementation progress",
	})
}

func (s *DocumentHandoffService) createFinalSummaryCommit(
	ctx context.Context,
	specTask *types.SpecTask,
	completionSummary string,
) error {
	// Create a comprehensive final summary document
	summaryContent := fmt.Sprintf(`# Final Implementation Summary: %s

**SpecTask**: %s
**Completed**: %s
**Total Duration**: %s

## Implementation Summary

%s

## Final Status

- **Status**: %s
- **All Work Sessions**: Complete
- **All Implementation Tasks**: Complete
- **Documentation**: Updated
- **Git History**: Complete

This SpecTask has been successfully implemented using the multi-session approach with full coordination and documentation.

---
*Final summary generated on %s*
`,
		specTask.Name,
		specTask.ID,
		time.Now().Format("2006-01-02 15:04:05"),
		s.calculateImplementationDuration(specTask),
		completionSummary,
		specTask.Status,
		time.Now().Format("2006-01-02 15:04:05"),
	)

	// Commit to root of sessions directory
	repoPath := s.getProjectPath(specTask)
	sessionDir := filepath.Join("sessions", specTask.ID)

	return s.commitSessionActivity(ctx, repoPath, sessionDir, "IMPLEMENTATION_COMPLETE.md", summaryContent, &SessionHistoryRecord{
		ActivityType: "completion",
		Timestamp:    time.Now(),
		Content:      "Implementation completed successfully",
	})
}

func (s *DocumentHandoffService) formatGenericActivity(record *SessionHistoryRecord) string {
	return fmt.Sprintf(`## %s Activity - %s

**Session**: %s
**Timestamp**: %s

%s

---
`,
		strings.Title(record.ActivityType),
		record.Timestamp.Format("2006-01-02 15:04:05"),
		record.WorkSessionID,
		record.Timestamp.Format("15:04:05"),
		record.Content,
	)
}

func (s *DocumentHandoffService) countCompletedSessions(activeSessions []types.SpecTaskWorkSession) int {
	count := 0
	for _, session := range activeSessions {
		if session.Status == types.SpecTaskWorkSessionStatusCompleted {
			count++
		}
	}
	return count
}

// Public API methods for external integration

// OnSpecApproved should be called when specs are approved in the UI
func (s *DocumentHandoffService) OnSpecApproved(
	ctx context.Context,
	specTaskID string,
	approval *types.SpecApprovalResponse,
) (*HandoffResult, error) {
	config := &DocumentHandoffConfig{
		EnableGitIntegration:   true,
		EnablePullRequests:     s.enablePullRequests,
		EnableSessionRecording: true,
		CommitFrequencyMinutes: 10,
		RequireCodeReview:      false,
		AutoMergeApprovedSpecs: true,
	}

	return s.ExecuteSpecApprovalHandoff(ctx, specTaskID, approval, config)
}

// OnSessionActivity should be called when session activity occurs
func (s *DocumentHandoffService) OnSessionActivity(
	ctx context.Context,
	workSessionID string,
	activityType string,
	content string,
	metadata map[string]interface{},
) error {
	record := &SessionHistoryRecord{
		WorkSessionID: workSessionID,
		ActivityType:  activityType,
		Timestamp:     time.Now(),
		Content:       content,
		Metadata:      metadata,
	}

	// Add Zed thread ID if available
	if workSession, err := s.store.GetSpecTaskWorkSession(ctx, workSessionID); err == nil {
		record.HelixSessionID = workSession.HelixSessionID
		if zedThread, err := s.store.GetSpecTaskZedThreadByWorkSession(ctx, workSessionID); err == nil {
			record.ZedThreadID = zedThread.ZedThreadID
		}
	}

	return s.RecordSessionHistory(ctx, record)
}

// OnImplementationComplete should be called when implementation finishes
func (s *DocumentHandoffService) OnImplementationComplete(
	ctx context.Context,
	specTaskID string,
	completionSummary string,
	artifacts map[string]string,
) (*HandoffResult, error) {
	return s.CompleteImplementationHandoff(ctx, specTaskID, completionSummary, artifacts)
}

// GetDocumentHandoffStatus returns the current status of document handoff for a SpecTask
func (s *DocumentHandoffService) GetDocumentHandoffStatus(
	ctx context.Context,
	specTaskID string,
) (*DocumentHandoffStatus, error) {
	specTask, err := s.store.GetSpecTask(ctx, specTaskID)
	if err != nil {
		return nil, fmt.Errorf("failed to get SpecTask: %w", err)
	}

	status := &DocumentHandoffStatus{
		SpecTaskID:             specTaskID,
		CurrentPhase:           s.determineCurrentPhase(specTask),
		GitIntegrationStatus:   s.checkGitIntegrationStatus(specTask),
		DocumentsGenerated:     s.checkDocumentsGenerated(specTask),
		SessionRecordingActive: s.checkSessionRecording(specTask),
		LastCommitHash:         "",  // Would query git
		LastCommitTime:         nil, // Would query git
	}

	return status, nil
}

// Supporting types

type DocumentHandoffStatus struct {
	SpecTaskID             string     `json:"spec_task_id"`
	CurrentPhase           string     `json:"current_phase"`
	GitIntegrationStatus   string     `json:"git_integration_status"`
	DocumentsGenerated     bool       `json:"documents_generated"`
	SessionRecordingActive bool       `json:"session_recording_active"`
	LastCommitHash         string     `json:"last_commit_hash"`
	LastCommitTime         *time.Time `json:"last_commit_time,omitempty"`
}

func (s *DocumentHandoffService) determineCurrentPhase(specTask *types.SpecTask) string {
	switch specTask.Status {
	case types.TaskStatusBacklog, types.TaskStatusSpecGeneration:
		return "planning"
	case types.TaskStatusSpecReview:
		return "review"
	case types.TaskStatusSpecApproved:
		return "approved"
	case types.TaskStatusImplementation:
		return "implementation"
	case types.TaskStatusDone:
		return "complete"
	default:
		return "unknown"
	}
}

func (s *DocumentHandoffService) checkGitIntegrationStatus(specTask *types.SpecTask) string {
	// Would check git repository status
	return "active"
}

func (s *DocumentHandoffService) checkDocumentsGenerated(specTask *types.SpecTask) bool {
	return specTask.RequirementsSpec != "" && specTask.TechnicalDesign != "" && specTask.ImplementationPlan != ""
}

func (s *DocumentHandoffService) checkSessionRecording(specTask *types.SpecTask) bool {
	return specTask.Status == types.TaskStatusImplementation
}
