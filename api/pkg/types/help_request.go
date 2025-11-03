package types

import (
	"time"

	"gorm.io/datatypes"
)

// SessionStatus represents the status of an agent session
type SessionStatus string

const (
	SessionStatusStarting       SessionStatus = "starting"
	SessionStatusActive         SessionStatus = "active"
	SessionStatusWaitingForHelp SessionStatus = "waiting_for_help"
	SessionStatusPaused         SessionStatus = "paused"
	SessionStatusCompleted      SessionStatus = "completed"
	SessionStatusPendingReview  SessionStatus = "pending_review"
	SessionStatusFailed         SessionStatus = "failed"
)

// HelpRequest represents a request for human assistance from an AI agent
type HelpRequest struct {
	ID                  string         `json:"id" gorm:"primaryKey"`
	SessionID           string         `json:"session_id" gorm:"index"`
	InteractionID       string         `json:"interaction_id" gorm:"index"`
	UserID              string         `json:"user_id" gorm:"index"`
	AppID               string         `json:"app_id" gorm:"index"`
	HelpType            string         `json:"help_type"`            // "decision", "expertise", "clarification", "review", "guidance", "stuck", "other"
	Context             string         `json:"context"`              // Brief context about the current task
	SpecificNeed        string         `json:"specific_need"`        // Specific description of what help is needed
	AttemptedSolutions  string         `json:"attempted_solutions"`  // What the agent has already tried
	Urgency             string         `json:"urgency"`              // "low", "medium", "high", "critical"
	SuggestedApproaches string         `json:"suggested_approaches"` // Potential approaches the agent suggests
	Status              string         `json:"status"`               // "pending", "in_progress", "resolved", "cancelled"
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
	ResolvedAt          *time.Time     `json:"resolved_at,omitempty"`
	ResolvedBy          string         `json:"resolved_by,omitempty"` // UserID of the human who resolved this
	Resolution          string         `json:"resolution,omitempty"`  // The resolution or guidance provided
	Metadata            datatypes.JSON `json:"metadata,omitempty"`    // Additional metadata as JSON
}

// AgentSession represents an active agent session with enhanced tracking
type AgentSession struct {
	ID          string         `json:"id" gorm:"primaryKey"`
	SessionID   string         `json:"session_id" gorm:"uniqueIndex"` // Maps to Helix session ID
	AgentType   string         `json:"agent_type"`                    // "zed", "helix", etc.
	Status      string         `json:"status"`                        // "starting", "active", "waiting_for_help", "paused", "completed", "failed"
	CurrentTask string         `json:"current_task"`                  // Description of what the agent is currently doing
	WorkItemID  string         `json:"work_item_id"`                  // Associated work item if any
	UserID      string         `json:"user_id" gorm:"index"`
	AppID       string         `json:"app_id" gorm:"index"`
	Config      datatypes.JSON `json:"config"` // Agent configuration
	State       datatypes.JSON `json:"state"`  // Current agent state/context

	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	LastActivity time.Time      `json:"last_activity"` // Last time the agent was active
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	ProcessID    int            `json:"process_id,omitempty"`   // OS process ID if applicable
	ContainerID  string         `json:"container_id,omitempty"` // Container ID if running in container
	HealthStatus string         `json:"health_status"`          // "healthy", "unhealthy", "unknown"
	Metadata     datatypes.JSON `json:"metadata,omitempty"`
}

// HelpRequestsListResponse represents the response for listing help requests
type HelpRequestsListResponse struct {
	HelpRequests []*HelpRequest `json:"help_requests"`
	Total        int            `json:"total"`
	Page         int            `json:"page"`
	PageSize     int            `json:"page_size"`
}

// AgentSessionsListResponse represents the response for listing agent sessions
type AgentSessionsListResponse struct {
	Sessions []*AgentSession `json:"sessions"`
	Total    int             `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// AgentDashboardData extends DashboardData to include agent information
type AgentDashboardData struct {
	*DashboardData
	ActiveSessions      []*AgentSessionStatus `json:"active_sessions"`
	PendingWork         []*AgentWorkItem      `json:"pending_work"`
	HelpRequests        []*HelpRequest        `json:"help_requests"`
	SessionsNeedingHelp []*AgentSessionStatus `json:"sessions_needing_help"`
	RecentCompletions   []*JobCompletion      `json:"recent_completions"`
	PendingReviews      []*JobCompletion      `json:"pending_reviews"`
}

// JobCompletion represents a completed job/task from an AI agent
type JobCompletion struct {
	ID               string         `json:"id" gorm:"primaryKey"`
	SessionID        string         `json:"session_id" gorm:"index"`
	InteractionID    string         `json:"interaction_id" gorm:"index"`
	UserID           string         `json:"user_id" gorm:"index"`
	AppID            string         `json:"app_id" gorm:"index"`
	WorkItemID       string         `json:"work_item_id,omitempty" gorm:"index"`
	CompletionStatus string         `json:"completion_status"` // "fully_completed", "milestone_reached", etc.
	Summary          string         `json:"summary"`
	Deliverables     string         `json:"deliverables"`
	ReviewNeeded     bool           `json:"review_needed"`
	ReviewType       string         `json:"review_type"` // "approval", "feedback", "validation", etc.
	NextSteps        string         `json:"next_steps"`
	Limitations      string         `json:"limitations"`
	FilesCreated     string         `json:"files_created"`
	TimeSpent        string         `json:"time_spent"`
	Confidence       string         `json:"confidence"` // "high", "medium", "low"
	Status           string         `json:"status"`     // "pending_review", "approved", "needs_changes", "archived"
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	ReviewedAt       *time.Time     `json:"reviewed_at,omitempty"`
	ReviewedBy       string         `json:"reviewed_by,omitempty"`
	ReviewNotes      string         `json:"review_notes,omitempty"`
	Metadata         datatypes.JSON `json:"metadata,omitempty"`
}

// JobCompletionsListResponse represents the response for listing job completions
type JobCompletionsListResponse struct {
	JobCompletions []*JobCompletion `json:"job_completions"`
	Total          int              `json:"total"`
	Page           int              `json:"page"`
	PageSize       int              `json:"page_size"`
}
