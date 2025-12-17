package types

import "time"

// AuditEventType represents the type of audit event
type AuditEventType string

const (
	// Task lifecycle events
	AuditEventTaskCreated    AuditEventType = "task_created"
	AuditEventTaskCloned     AuditEventType = "task_cloned" // Task cloned from another task
	AuditEventTaskApproved   AuditEventType = "task_approved"
	AuditEventTaskCompleted  AuditEventType = "task_completed"
	AuditEventTaskArchived   AuditEventType = "task_archived"
	AuditEventTaskUnarchived AuditEventType = "task_unarchived"

	// Agent interaction events
	AuditEventAgentPrompt  AuditEventType = "agent_prompt"  // Prompt sent from Helix UI to agent
	AuditEventUserMessage  AuditEventType = "user_message"  // Message sent by user inside agent (via WebSocket)
	AuditEventAgentStarted AuditEventType = "agent_started" // Agent session started

	// Spec events
	AuditEventSpecGenerated AuditEventType = "spec_generated" // Spec was generated
	AuditEventSpecUpdated   AuditEventType = "spec_updated"   // Spec was modified

	// Design review events
	AuditEventReviewComment      AuditEventType = "review_comment"       // Comment added to design review
	AuditEventReviewCommentReply AuditEventType = "review_comment_reply" // Reply to a comment

	// Git/PR events
	AuditEventPRCreated AuditEventType = "pr_created" // Pull request created
	AuditEventPRMerged  AuditEventType = "pr_merged"  // Pull request merged
	AuditEventGitPush   AuditEventType = "git_push"   // Git push detected

	// Project lifecycle events
	AuditEventProjectCreated           AuditEventType = "project_created"            // Project was created
	AuditEventProjectDeleted           AuditEventType = "project_deleted"            // Project was deleted
	AuditEventProjectSettingsUpdated   AuditEventType = "project_settings_updated"   // Project settings were modified
	AuditEventProjectGuidelinesUpdated AuditEventType = "project_guidelines_updated" // Project guidelines were modified
)

// AuditMetadata contains additional context for audit log entries
type AuditMetadata struct {
	// Project information
	ProjectName string `json:"project_name,omitempty"`

	// Task information
	TaskNumber int    `json:"task_number,omitempty"`
	TaskName   string `json:"task_name,omitempty"`
	BranchName string `json:"branch_name,omitempty"`

	// Helix session/interaction linking
	SessionID     string `json:"session_id,omitempty"`
	InteractionID string `json:"interaction_id,omitempty"` // For scrolling to specific interaction in session view

	// Pull request information
	PullRequestID  string `json:"pull_request_id,omitempty"`
	PullRequestURL string `json:"pull_request_url,omitempty"`

	// External system tracking (e.g., Azure DevOps)
	ExternalTaskID  string `json:"external_task_id,omitempty"`
	ExternalTaskURL string `json:"external_task_url,omitempty"`

	// Spec versioning - capture spec state at time of event
	SpecVersion           int    `json:"spec_version,omitempty"`            // Version number of spec at time of event
	RequirementsSpecHash  string `json:"requirements_spec_hash,omitempty"`  // Hash of requirements spec content
	TechnicalDesignHash   string `json:"technical_design_hash,omitempty"`   // Hash of technical design content
	ImplementationPlanHash string `json:"implementation_plan_hash,omitempty"` // Hash of implementation plan content

	// Design review tracking
	DesignReviewID string `json:"design_review_id,omitempty"`
	CommentID      string `json:"comment_id,omitempty"`

	// Clone tracking (matches SpecTask fields)
	ClonedFromID        string `json:"cloned_from_id,omitempty"`         // Source task ID if this was cloned
	ClonedFromProjectID string `json:"cloned_from_project_id,omitempty"` // Source project ID if cloned from another project
	CloneGroupID        string `json:"clone_group_id,omitempty"`         // Group ID linking related cloned tasks
}

// ProjectAuditLog represents an audit trail entry for a project
// This table is append-only - entries are never updated or deleted
type ProjectAuditLog struct {
	ID         string         `json:"id" gorm:"primaryKey;size:255"`
	ProjectID  string         `json:"project_id" gorm:"size:255;index;not null"`
	SpecTaskID string         `json:"spec_task_id,omitempty" gorm:"size:255;index"`
	UserID     string         `json:"user_id" gorm:"size:255;not null"`
	UserEmail  string         `json:"user_email,omitempty" gorm:"size:255"`
	EventType  AuditEventType `json:"event_type" gorm:"size:50;index;not null"`
	PromptText string         `json:"prompt_text,omitempty" gorm:"type:text"`
	Metadata   AuditMetadata  `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`
	CreatedAt  time.Time      `json:"created_at" gorm:"index"`
}

// ProjectAuditLogFilters for filtering audit log queries
type ProjectAuditLogFilters struct {
	ProjectID  string         `json:"project_id"`
	EventType  AuditEventType `json:"event_type,omitempty"`
	UserID     string         `json:"user_id,omitempty"`
	SpecTaskID string         `json:"spec_task_id,omitempty"`
	StartDate  *time.Time     `json:"start_date,omitempty"`
	EndDate    *time.Time     `json:"end_date,omitempty"`
	Search     string         `json:"search,omitempty"`
	Limit      int            `json:"limit,omitempty"`
	Offset     int            `json:"offset,omitempty"`
}

// ProjectAuditLogResponse represents the paginated response for audit logs
type ProjectAuditLogResponse struct {
	Logs   []*ProjectAuditLog `json:"logs"`
	Total  int64              `json:"total"`
	Limit  int                `json:"limit"`
	Offset int                `json:"offset"`
}
