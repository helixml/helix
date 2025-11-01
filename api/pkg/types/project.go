package types

import (
	"time"

	"gorm.io/datatypes"
)

// Project represents a Helix project that can contain tasks and agent work
type Project struct {
	ID             string         `json:"id" gorm:"primaryKey"`
	Name           string         `json:"name"`
	Description    string         `json:"description"`
	UserID         string         `json:"user_id" gorm:"index"`
	OrganizationID string         `json:"organization_id" gorm:"index"`
	GitHubRepoURL string   `json:"github_repo_url"`
	DefaultBranch string   `json:"default_branch"`
	Technologies  []string `json:"technologies" gorm:"type:jsonb;serializer:json"`
	Status        string   `json:"status"` // "active", "archived", "completed"

	// Project-level repository management
	DefaultRepoID  string `json:"default_repo_id" gorm:"type:varchar(255)"` // Primary repository for the project

	// Per-project startup script
	StartupScript  string `json:"startup_script" gorm:"type:text"` // Bash script to run when agent starts

	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	Metadata       datatypes.JSON `json:"metadata,omitempty"`
}

// ProjectTask represents a task within a project (extends AgentWorkItem for project-specific tasks)
type ProjectTask struct {
	ID                   string         `json:"id" gorm:"primaryKey"`
	ProjectID            string         `json:"project_id" gorm:"index"`
	Name                 string         `json:"name"`
	Description          string         `json:"description"`
	Type                 string         `json:"type"`     // "feature", "bug", "task", "epic"
	Priority             string         `json:"priority"` // "low", "medium", "high", "critical"
	Status               string         `json:"status"`   // "backlog", "ready", "in_progress", "review", "done"
	AssignedAgent        string         `json:"assigned_agent,omitempty"`
	SessionID            string         `json:"session_id,omitempty"`
	BranchName           string         `json:"branch_name,omitempty"`
	EstimatedHours     int      `json:"estimated_hours,omitempty"`
	ActualHours        int      `json:"actual_hours,omitempty"`
	Labels             []string `json:"labels" gorm:"type:jsonb;serializer:json"`
	AcceptanceCriteria []string `json:"acceptance_criteria" gorm:"type:jsonb;serializer:json"`
	TechnicalNotes     string   `json:"technical_notes,omitempty"`
	FilesToModify      []string `json:"files_to_modify" gorm:"type:jsonb;serializer:json"`
	CreatedAt            time.Time      `json:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at"`
	CreatedBy            string         `json:"created_by"`
	DueDate              *time.Time     `json:"due_date,omitempty"`
	CompletedAt          *time.Time     `json:"completed_at,omitempty"`

	// GitHub integration
	GitHubIssue *ProjectTaskGitHubIssue `json:"github_issue,omitempty" gorm:"embedded;embeddedPrefix:github_issue_"`
	PullRequest *ProjectTaskPullRequest `json:"pull_request,omitempty" gorm:"embedded;embeddedPrefix:pr_"`

	// Agent progress tracking
	AgentProgress *ProjectTaskAgentProgress `json:"agent_progress,omitempty" gorm:"embedded;embeddedPrefix:agent_progress_"`

	Metadata datatypes.JSON `json:"metadata,omitempty"`
}

// ProjectTaskGitHubIssue represents GitHub issue integration
type ProjectTaskGitHubIssue struct {
	Number int    `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
}

// ProjectTaskPullRequest represents pull request information
type ProjectTaskPullRequest struct {
	Number int    `json:"number,omitempty"`
	URL    string `json:"url,omitempty"`
	Status string `json:"status,omitempty"` // "draft", "open", "merged", "closed"
}

// ProjectTaskAgentProgress tracks agent progress on a task
type ProjectTaskAgentProgress struct {
	CompletedSteps  []string   `json:"completed_steps" gorm:"type:jsonb;serializer:json"`
	CurrentStep     string     `json:"current_step,omitempty"`
	Blockers        []string   `json:"blockers" gorm:"type:jsonb;serializer:json"`
	ProgressPercent int        `json:"progress_percent,omitempty"`
	LastUpdateAt    *time.Time `json:"last_update_at,omitempty"`
}

// ProjectStats represents project statistics
type ProjectStats struct {
	TotalTasks      int            `json:"total_tasks"`
	CompletedTasks  int            `json:"completed_tasks"`
	InProgressTasks int            `json:"in_progress_tasks"`
	TasksByStatus   map[string]int `json:"tasks_by_status"`
	TasksByPriority map[string]int `json:"tasks_by_priority"`
	TasksByType     map[string]int `json:"tasks_by_type"`
	AgentSessions   int            `json:"active_agent_sessions"`
	AverageTaskTime float64        `json:"average_task_completion_hours"`
}

// ProjectsListResponse represents the response for listing projects
type ProjectsListResponse struct {
	Projects []*Project `json:"projects"`
	Total    int        `json:"total"`
	Page     int        `json:"page"`
	PageSize int        `json:"page_size"`
}

// ProjectTasksResponse represents the response for project tasks
type ProjectTasksResponse struct {
	Tasks    []*ProjectTask `json:"tasks"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"page_size"`
}

// ProjectCreateRequest represents a request to create a new project
type ProjectCreateRequest struct {
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	GitHubRepoURL string   `json:"github_repo_url,omitempty"`
	DefaultBranch string   `json:"default_branch,omitempty"`
	Technologies  []string `json:"technologies,omitempty"`
	DefaultRepoID string   `json:"default_repo_id,omitempty"`
	StartupScript string   `json:"startup_script,omitempty"`
}

// ProjectUpdateRequest represents a request to update a project
type ProjectUpdateRequest struct {
	Name          *string  `json:"name,omitempty"`
	Description   *string  `json:"description,omitempty"`
	GitHubRepoURL *string  `json:"github_repo_url,omitempty"`
	DefaultBranch *string  `json:"default_branch,omitempty"`
	Technologies  []string `json:"technologies,omitempty"`
	Status        *string  `json:"status,omitempty"`
	DefaultRepoID *string  `json:"default_repo_id,omitempty"`
	StartupScript *string  `json:"startup_script,omitempty"`
}

// ProjectTaskCreateRequest represents a request to create a new project task
type ProjectTaskCreateRequest struct {
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Type               string     `json:"type"`
	Priority           string     `json:"priority"`
	Status             string     `json:"status,omitempty"`
	EstimatedHours     int        `json:"estimated_hours,omitempty"`
	Labels             []string   `json:"labels,omitempty"`
	AcceptanceCriteria []string   `json:"acceptance_criteria,omitempty"`
	TechnicalNotes     string     `json:"technical_notes,omitempty"`
	FilesToModify      []string   `json:"files_to_modify,omitempty"`
	DueDate            *time.Time `json:"due_date,omitempty"`
}

// ProjectTaskUpdateRequest represents a request to update a project task
type ProjectTaskUpdateRequest struct {
	Name               *string    `json:"name,omitempty"`
	Description        *string    `json:"description,omitempty"`
	Type               *string    `json:"type,omitempty"`
	Priority           *string    `json:"priority,omitempty"`
	Status             *string    `json:"status,omitempty"`
	EstimatedHours     *int       `json:"estimated_hours,omitempty"`
	ActualHours        *int       `json:"actual_hours,omitempty"`
	Labels             []string   `json:"labels,omitempty"`
	AcceptanceCriteria []string   `json:"acceptance_criteria,omitempty"`
	TechnicalNotes     *string    `json:"technical_notes,omitempty"`
	FilesToModify      []string   `json:"files_to_modify,omitempty"`
	DueDate            *time.Time `json:"due_date,omitempty"`
}

// ProjectDashboard represents a comprehensive project dashboard view
type ProjectDashboard struct {
	Project        *Project              `json:"project"`
	Stats          *ProjectStats         `json:"stats"`
	RecentTasks    []*ProjectTask        `json:"recent_tasks"`
	ActiveSessions []*AgentSessionStatus `json:"active_sessions"`
	RecentActivity []ProjectActivity     `json:"recent_activity"`
	KanbanColumns  []ProjectKanbanColumn `json:"kanban_columns"`
}

// ProjectActivity represents activity log entries for a project
type ProjectActivity struct {
	ID        string         `json:"id"`
	ProjectID string         `json:"project_id"`
	TaskID    string         `json:"task_id,omitempty"`
	UserID    string         `json:"user_id"`
	AgentType string         `json:"agent_type,omitempty"`
	Action    string         `json:"action"` // "task_created", "task_moved", "agent_assigned", "task_completed", etc.
	Details   string         `json:"details"`
	Timestamp time.Time      `json:"timestamp"`
	Metadata  datatypes.JSON `json:"metadata,omitempty"`
}

// ProjectKanbanColumn represents a column in the project's kanban board
type ProjectKanbanColumn struct {
	ID       string         `json:"id"`
	Title    string         `json:"title"`
	Color    string         `json:"color"`
	Position int            `json:"position"`
	WIPLimit int            `json:"wip_limit,omitempty"`
	Tasks    []*ProjectTask `json:"tasks"`
}

// ProjectTemplate represents a project template for quick project creation
type ProjectTemplate struct {
	ID            string                `json:"id"`
	Name          string                `json:"name"`
	Description   string                `json:"description"`
	Category      string                `json:"category"`
	Technologies  []string              `json:"technologies"`
	TaskTemplates []ProjectTaskTemplate `json:"task_templates"`
	GitHubRepo    string                `json:"github_repo,omitempty"`
	ReadmeURL     string                `json:"readme_url,omitempty"`
	DemoURL       string                `json:"demo_url,omitempty"`
}

// ProjectTaskTemplate represents a task template within a project template
type ProjectTaskTemplate struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Type               string   `json:"type"`
	Priority           string   `json:"priority"`
	EstimatedHours     int      `json:"estimated_hours,omitempty"`
	Labels             []string `json:"labels,omitempty"`
	AcceptanceCriteria []string `json:"acceptance_criteria,omitempty"`
	TechnicalNotes     string   `json:"technical_notes,omitempty"`
	FilesToModify      []string `json:"files_to_modify,omitempty"`
}

// SampleProject represents a pre-built sample project that can be instantiated
type SampleProject struct {
	ID            string         `json:"id" gorm:"primaryKey;type:varchar(255)"`
	Name          string         `json:"name" gorm:"type:varchar(255);not null"`
	Description   string         `json:"description" gorm:"type:text"`
	Category      string         `json:"category" gorm:"type:varchar(100)"` // 'web', 'mobile', 'api', 'ml', etc.
	Difficulty    string         `json:"difficulty" gorm:"type:varchar(50)"` // 'beginner', 'intermediate', 'advanced'
	RepositoryURL string         `json:"repository_url" gorm:"type:text;not null"`
	StartupScript string         `json:"startup_script" gorm:"type:text"`
	ThumbnailURL  string         `json:"thumbnail_url" gorm:"type:text"`
	SampleTasks   datatypes.JSON `json:"sample_tasks" gorm:"type:jsonb"` // Array of {title, description, priority, type}
	CreatedAt     time.Time      `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
}

// SampleProjectTask represents a pre-defined task for a sample project
type SampleProjectTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
}

// SampleProjectInstantiateRequest represents a request to instantiate a sample project
type SampleProjectInstantiateRequest struct {
	ProjectName string `json:"project_name,omitempty"` // Optional custom name for the instantiated project
}

// SampleProjectInstantiateResponse represents the response after instantiating a sample project
type SampleProjectInstantiateResponse struct {
	ProjectID string `json:"project_id"`
	Message   string `json:"message"`
}
