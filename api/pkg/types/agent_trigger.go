package types

import (
	"time"

	"gorm.io/datatypes"
)

// AgentWorkQueueTrigger represents a trigger for agent work queue items
type AgentWorkQueueTrigger struct {
	Enabled     bool   `json:"enabled" yaml:"enabled"`
	AgentType   string `json:"agent_type" yaml:"agent_type"`     // "zed", "helix", etc.
	Priority    int    `json:"priority" yaml:"priority"`         // Lower numbers = higher priority
	MaxRetries  int    `json:"max_retries" yaml:"max_retries"`   // Maximum retry attempts
	TimeoutMins int    `json:"timeout_mins" yaml:"timeout_mins"` // Timeout in minutes
	AutoAssign  bool   `json:"auto_assign" yaml:"auto_assign"`   // Auto-assign to available agents

	// Work item configuration
	WorkConfig AgentWorkConfig `json:"work_config" yaml:"work_config"`
}

// AgentWorkConfig defines the configuration for agent work items
type AgentWorkConfig struct {
	// Source integration settings
	GitHub  *GitHubWorkConfig  `json:"github,omitempty" yaml:"github,omitempty"`
	Manual  *ManualWorkConfig  `json:"manual,omitempty" yaml:"manual,omitempty"`
	Webhook *WebhookWorkConfig `json:"webhook,omitempty" yaml:"webhook,omitempty"`

	// Agent environment and setup
	Environment map[string]string `json:"environment,omitempty" yaml:"environment,omitempty"`
	WorkingDir  string            `json:"working_dir,omitempty" yaml:"working_dir,omitempty"`

	// Skills to enable for this work type
	Skills []string `json:"skills,omitempty" yaml:"skills,omitempty"`

	// Custom instructions or context
	Instructions string `json:"instructions,omitempty" yaml:"instructions,omitempty"`
}

// GitHubWorkConfig for GitHub issue/PR work items
type GitHubWorkConfig struct {
	Enabled     bool     `json:"enabled" yaml:"enabled"`
	RepoOwner   string   `json:"repo_owner" yaml:"repo_owner"`
	RepoName    string   `json:"repo_name" yaml:"repo_name"`
	Labels      []string `json:"labels,omitempty" yaml:"labels,omitempty"`           // Filter by labels
	IssueTypes  []string `json:"issue_types,omitempty" yaml:"issue_types,omitempty"` // "issue", "pull_request"
	AutoComment bool     `json:"auto_comment" yaml:"auto_comment"`                   // Comment when starting work
	AccessToken string   `json:"access_token,omitempty" yaml:"access_token,omitempty"`
}

// ManualWorkConfig for manually created work items
type ManualWorkConfig struct {
	Enabled         bool `json:"enabled" yaml:"enabled"`
	AllowAnonymous  bool `json:"allow_anonymous" yaml:"allow_anonymous"`
	DefaultPriority int  `json:"default_priority" yaml:"default_priority"`
}

// WebhookWorkConfig for webhook-triggered work items
type WebhookWorkConfig struct {
	Enabled  bool              `json:"enabled" yaml:"enabled"`
	Secret   string            `json:"secret,omitempty" yaml:"secret,omitempty"`
	Headers  map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
	JSONPath string            `json:"json_path,omitempty" yaml:"json_path,omitempty"` // JSONPath to extract work description
}

// AgentWorkExecution represents the execution of a work item by an agent
type AgentWorkExecution struct {
	ID              string         `json:"id" gorm:"primaryKey"`
	TriggerConfigID string         `json:"trigger_config_id" gorm:"index"`
	WorkItemID      string         `json:"work_item_id" gorm:"index"`
	SessionID       string         `json:"session_id" gorm:"index"`
	AgentType       string         `json:"agent_type"`
	Status          string         `json:"status"` // "pending", "running", "completed", "failed", "cancelled"
	StartedAt       *time.Time     `json:"started_at,omitempty"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Duration        int            `json:"duration"` // Duration in seconds
	RetryCount      int            `json:"retry_count"`
	LastError       string         `json:"last_error,omitempty"`
	Result          string         `json:"result,omitempty"` // Final result or output
	Logs            string         `json:"logs,omitempty"`   // Execution logs
	WorkData        datatypes.JSON `json:"work_data"`        // Original work data
	ExecutionData   datatypes.JSON `json:"execution_data"`   // Execution-specific data
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

// AgentWorkItem represents a work item in the agent queue (extends the existing system)
type AgentWorkItem struct {
	ID                string         `json:"id" gorm:"primaryKey"`
	TriggerConfigID   string         `json:"trigger_config_id" gorm:"index"` // Links to TriggerConfiguration
	Name              string         `json:"name"`
	Description       string         `json:"description"`
	Source            string         `json:"source"`     // "github", "manual", "webhook", etc.
	SourceID          string         `json:"source_id"`  // External ID from source system
	SourceURL         string         `json:"source_url"` // URL to source (e.g., GitHub issue URL)
	Priority          int            `json:"priority"`   // Lower = higher priority
	Status            string         `json:"status"`     // "pending", "assigned", "in_progress", "completed", "failed", "cancelled"
	AgentType         string         `json:"agent_type"` // Required agent type
	AssignedSessionID string         `json:"assigned_session_id,omitempty" gorm:"index"`
	UserID            string                 `json:"user_id" gorm:"index"`
	AppID             string                 `json:"app_id" gorm:"index"`
	OrganizationID    string                 `json:"organization_id" gorm:"index"`
	WorkData          map[string]interface{} `json:"work_data" gorm:"type:jsonb;serializer:json"` // Work-specific data
	Config            map[string]interface{} `json:"config" gorm:"type:jsonb;serializer:json"`    // Agent configuration
	Labels            []string               `json:"labels" gorm:"type:jsonb;serializer:json"`    // Labels/tags for filtering
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
	ScheduledFor      *time.Time             `json:"scheduled_for,omitempty"` // When to start this work
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	DeadlineAt        *time.Time             `json:"deadline_at,omitempty"`
	MaxRetries        int                    `json:"max_retries"`
	RetryCount        int                    `json:"retry_count"`
	LastError         string                 `json:"last_error,omitempty"`
	Metadata          map[string]interface{} `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`
}

// AgentSessionStatus represents enhanced status for agent sessions
type AgentSessionStatus struct {
	ID              string         `json:"id" gorm:"primaryKey"`
	SessionID       string         `json:"session_id" gorm:"uniqueIndex"`
	AgentType       string         `json:"agent_type"`
	Status          string         `json:"status"`       // "starting", "active", "waiting_for_help", "paused", "completed", "pending_review", "failed"
	CurrentTask     string         `json:"current_task"` // What the agent is currently doing
	CurrentWorkItem string         `json:"current_work_item,omitempty" gorm:"index"`
	UserID          string         `json:"user_id" gorm:"index"`
	AppID           string         `json:"app_id" gorm:"index"`
	OrganizationID  string         `json:"organization_id" gorm:"index"`
	ProcessID       int            `json:"process_id,omitempty"`
	ContainerID     string         `json:"container_id,omitempty"`
	RDPPort         int            `json:"rdp_port,omitempty"`
	WorkspaceDir    string         `json:"workspace_dir,omitempty"`
	HealthStatus    string         `json:"health_status"` // "healthy", "unhealthy", "unknown"
	HealthCheckedAt *time.Time     `json:"health_checked_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
	LastActivity    time.Time      `json:"last_activity"`
	CompletedAt     *time.Time     `json:"completed_at,omitempty"`
	Configuration   datatypes.JSON `json:"configuration"`
	State           datatypes.JSON `json:"state"`
	Metrics         datatypes.JSON `json:"metrics"`
	Metadata        datatypes.JSON `json:"metadata,omitempty"`
}

// Add the agent work queue trigger type to the existing TriggerType constants
const (
	TriggerTypeAgentWorkQueue TriggerType = "agent_work_queue"
)

// Extend the existing Trigger struct to include agent work queue
// Note: This should be added to the existing Trigger struct in types.go
// type Trigger struct {
//     Discord        *DiscordTrigger        `json:"discord,omitempty" yaml:"discord,omitempty"`
//     Slack          *SlackTrigger          `json:"slack,omitempty" yaml:"slack,omitempty"`
//     Cron           *CronTrigger           `json:"cron,omitempty" yaml:"cron,omitempty"`
//     AzureDevOps    *AzureDevOpsTrigger    `json:"azure_devops,omitempty" yaml:"azure_devops,omitempty"`
//     AgentWorkQueue *AgentWorkQueueTrigger `json:"agent_work_queue,omitempty" yaml:"agent_work_queue,omitempty"`
// }

// AgentWorkItemCreateRequest for creating new work items
type AgentWorkItemCreateRequest struct {
	Name         string                 `json:"name"`
	Description  string                 `json:"description"`
	Source       string                 `json:"source"`
	SourceID     string                 `json:"source_id,omitempty"`
	SourceURL    string                 `json:"source_url,omitempty"`
	Priority     int                    `json:"priority"`
	AgentType    string                 `json:"agent_type"`
	WorkData     map[string]interface{} `json:"work_data,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
	Labels       []string               `json:"labels,omitempty"`
	ScheduledFor *time.Time             `json:"scheduled_for,omitempty"`
	DeadlineAt   *time.Time             `json:"deadline_at,omitempty"`
	MaxRetries   int                    `json:"max_retries"`
}

// AgentWorkItemUpdateRequest for updating work items
type AgentWorkItemUpdateRequest struct {
	Name        *string                `json:"name,omitempty"`
	Description *string                `json:"description,omitempty"`
	Priority    *int                   `json:"priority,omitempty"`
	Status      *string                `json:"status,omitempty"`
	WorkData    map[string]interface{} `json:"work_data,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Labels      []string               `json:"labels,omitempty"`
}

// AgentWorkQueueStats for dashboard display
type AgentWorkQueueStats struct {
	TotalPending    int            `json:"total_pending"`
	TotalRunning    int            `json:"total_running"`
	TotalCompleted  int            `json:"total_completed"`
	TotalFailed     int            `json:"total_failed"`
	ActiveSessions  int            `json:"active_sessions"`
	ByAgentType     map[string]int `json:"by_agent_type"`
	BySource        map[string]int `json:"by_source"`
	ByPriority      map[string]int `json:"by_priority"`
	AverageWaitTime float64        `json:"average_wait_time_minutes"`
	OldestPending   *time.Time     `json:"oldest_pending,omitempty"`
}

// AgentDashboardSummary provides a comprehensive view of agent activity
type AgentDashboardSummary struct {
	*DashboardData                            // Embed existing dashboard data
	ActiveSessions      []*AgentSessionStatus `json:"active_sessions"`
	SessionsNeedingHelp []*AgentSessionStatus `json:"sessions_needing_help"`
	PendingWork         []*AgentWorkItem      `json:"pending_work"`
	RunningWork         []*AgentWorkItem      `json:"running_work"`
	RecentCompletions   []*JobCompletion      `json:"recent_completions"`
	PendingReviews      []*JobCompletion      `json:"pending_reviews"`
	ActiveHelpRequests  []*HelpRequest        `json:"active_help_requests"`
	WorkQueueStats      *AgentWorkQueueStats  `json:"work_queue_stats"`
	LastUpdated         time.Time             `json:"last_updated"`
}

// AgentFleetSummary contains only agent fleet data without embedded dashboard data
type AgentFleetSummary struct {
	ActiveSessions       []*AgentSessionStatus      `json:"active_sessions"`
	SessionsNeedingHelp  []*AgentSessionStatus      `json:"sessions_needing_help"`
	PendingWork          []*AgentWorkItem           `json:"pending_work"`
	RunningWork          []*AgentWorkItem           `json:"running_work"`
	RecentCompletions    []*JobCompletion           `json:"recent_completions"`
	PendingReviews       []*JobCompletion           `json:"pending_reviews"`
	ActiveHelpRequests   []*HelpRequest             `json:"active_help_requests"`
	WorkQueueStats       *AgentWorkQueueStats       `json:"work_queue_stats"`
	ExternalAgentRunners []*ExternalAgentConnection `json:"external_agent_runners"`
	LastUpdated          time.Time                  `json:"last_updated"`
}

// Response types for API endpoints
type AgentWorkItemsResponse struct {
	Items    []*AgentWorkItem `json:"items"`
	Total    int              `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

// AgentWorkItemsListResponse represents the response for listing work items
type AgentWorkItemsListResponse struct {
	WorkItems []*AgentWorkItem `json:"work_items"`
	Total     int              `json:"total"`
	Page      int              `json:"page"`
	PageSize  int              `json:"page_size"`
}

type AgentSessionsResponse struct {
	Sessions []*AgentSessionStatus `json:"sessions"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"page_size"`
}

type AgentWorkExecutionsResponse struct {
	Executions []*AgentWorkExecution `json:"executions"`
	Total      int                   `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
}
