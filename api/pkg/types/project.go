package types

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Project represents a Helix project that can contain tasks and agent work
type Project struct {
	ID             string   `json:"id" gorm:"primaryKey"`
	Name           string   `json:"name"`
	Description    string   `json:"description"`
	UserID         string   `json:"user_id" gorm:"index"`
	OrganizationID string   `json:"organization_id" gorm:"index"`
	GitHubRepoURL  string   `json:"github_repo_url"`
	DefaultBranch  string   `json:"default_branch"`
	Technologies   []string `json:"technologies" gorm:"type:jsonb;serializer:json"`
	Status         string   `json:"status"` // "active", "archived", "completed"

	// Project-level repository management
	// DefaultRepoID is the PRIMARY repository - startup script lives at .helix/startup.sh in this repo
	DefaultRepoID string `json:"default_repo_id"`

	// Transient field - loaded from primary code repo's .helix/startup.sh, never persisted to database
	StartupScript string `json:"startup_script" gorm:"-"`

	// Automation settings
	AutoStartBacklogTasks bool `json:"auto_start_backlog_tasks"` // Automatically move backlog tasks to planning when capacity available

	// Default agent for spec tasks in this project (App ID)
	// New spec tasks inherit this agent; can be overridden per-task
	DefaultHelixAppID string `json:"default_helix_app_id"`

	ProjectManagerHelixAppID string `json:"project_manager_helix_app_id"`

	PullRequestReviewerHelixAppID string `json:"pull_request_reviewer_helix_app_id"`
	PullRequestReviewsEnabled     bool   `json:"pull_request_reviews_enabled"`

	// Guidelines for AI agents - project-specific style guides, conventions, and instructions
	// Combined with organization guidelines when constructing prompts
	Guidelines          string    `json:"guidelines"`
	GuidelinesVersion   int       `json:"guidelines_version"`    // Incremented on each update
	GuidelinesUpdatedAt time.Time `json:"guidelines_updated_at"` // When guidelines were last updated
	GuidelinesUpdatedBy string    `json:"guidelines_updated_by"` // User ID who last updated guidelines

	// Auto-incrementing task number for human-readable directory names
	// Each SpecTask gets assigned the next number (install-cowsay_1, add-api_2, etc.)
	NextTaskNumber int `json:"next_task_number" gorm:"default:1"`

	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at,omitempty" gorm:"index"` // Soft delete timestamp
	Metadata  ProjectMetadata `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`
}

// ProjectTask represents a task within a project (extends AgentWorkItem for project-specific tasks)
type ProjectTask struct {
	ID                 string     `json:"id" gorm:"primaryKey"`
	ProjectID          string     `json:"project_id" gorm:"index"`
	Name               string     `json:"name"`
	Description        string     `json:"description"`
	Type               string     `json:"type"`     // "feature", "bug", "task", "epic"
	Priority           string     `json:"priority"` // "low", "medium", "high", "critical"
	Status             string     `json:"status"`   // "backlog", "ready", "in_progress", "review", "done"
	AssignedAgent      string     `json:"assigned_agent,omitempty"`
	SessionID          string     `json:"session_id,omitempty"`
	BranchName         string     `json:"branch_name,omitempty"`
	EstimatedHours     int        `json:"estimated_hours,omitempty"`
	ActualHours        int        `json:"actual_hours,omitempty"`
	Labels             []string   `json:"labels" gorm:"type:jsonb;serializer:json"`
	AcceptanceCriteria []string   `json:"acceptance_criteria" gorm:"type:jsonb;serializer:json"`
	TechnicalNotes     string     `json:"technical_notes,omitempty"`
	FilesToModify      []string   `json:"files_to_modify" gorm:"type:jsonb;serializer:json"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
	CreatedBy          string     `json:"created_by"`
	DueDate            *time.Time `json:"due_date,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`

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
	OrganizationID    string   `json:"organization_id"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	GitHubRepoURL     string   `json:"github_repo_url,omitempty"`
	DefaultBranch     string   `json:"default_branch,omitempty"`
	Technologies      []string `json:"technologies,omitempty"`
	DefaultRepoID     string   `json:"default_repo_id,omitempty"`
	StartupScript     string   `json:"startup_script,omitempty"`
	DefaultHelixAppID string   `json:"default_helix_app_id,omitempty"` // Default agent for spec tasks
	Guidelines        string   `json:"guidelines,omitempty"`           // Project-specific AI agent guidelines
}

// ProjectUpdateRequest represents a request to update a project
type ProjectUpdateRequest struct {
	Name                          *string          `json:"name,omitempty"`
	Description                   *string          `json:"description,omitempty"`
	GitHubRepoURL                 *string          `json:"github_repo_url,omitempty"`
	DefaultBranch                 *string          `json:"default_branch,omitempty"`
	Technologies                  []string         `json:"technologies,omitempty"`
	Status                        *string          `json:"status,omitempty"`
	DefaultRepoID                 *string          `json:"default_repo_id,omitempty"`
	StartupScript                 *string          `json:"startup_script,omitempty"`
	AutoStartBacklogTasks         *bool            `json:"auto_start_backlog_tasks,omitempty"`
	DefaultHelixAppID             *string          `json:"default_helix_app_id,omitempty"`               // Default agent for spec tasks
	ProjectManagerHelixAppID      *string          `json:"project_manager_helix_app_id,omitempty"`       // Project manager agent
	PullRequestReviewerHelixAppID *string          `json:"pull_request_reviewer_helix_app_id,omitempty"` // Pull request reviewer agent
	PullRequestReviewsEnabled     *bool            `json:"pull_request_reviews_enabled,omitempty"`       // Whether pull request reviews are enabled
	Guidelines                    *string          `json:"guidelines,omitempty"`                         // Project-specific AI agent guidelines
	Metadata                      *ProjectMetadata `json:"metadata,omitempty"`
}

// SampleProject represents a pre-built sample project that can be instantiated
type SampleProject struct {
	ID            string `json:"id" gorm:"primaryKey;type:varchar(255)"`
	Name          string `json:"name" gorm:"type:varchar(255);not null"`
	Description   string `json:"description" gorm:"type:text"`
	Category      string `json:"category" gorm:"type:varchar(100)"`  // 'web', 'mobile', 'api', 'ml', etc.
	Difficulty    string `json:"difficulty" gorm:"type:varchar(50)"` // 'beginner', 'intermediate', 'advanced'
	RepositoryURL string `json:"repository_url" gorm:"type:text;not null"`
	// NOTE: StartupScript is stored in the sample's Git repo at .helix/startup.sh, not in database
	ThumbnailURL string         `json:"thumbnail_url" gorm:"type:text"`
	SampleTasks  datatypes.JSON `json:"sample_tasks" gorm:"type:jsonb"` // Array of {title, description, priority, type}
	CreatedAt    time.Time      `json:"created_at" gorm:"default:CURRENT_TIMESTAMP"`
}

// SampleProjectTask represents a pre-defined task for a sample project
type SampleProjectTask struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Priority    string `json:"priority"`
	Type        string `json:"type"`
}

// ProjectMetadata represents the metadata stored in Project.Metadata field
type ProjectMetadata struct {
	BoardSettings *BoardSettings `json:"board_settings,omitempty"`
}

// BoardSettings represents the Kanban board settings for a project
type BoardSettings struct {
	WIPLimits WIPLimits `json:"wip_limits"`
}

// "planning":       3,
// "review":         2,
// "implementation": 5,
type WIPLimits struct {
	Planning       int `json:"planning"`
	Review         int `json:"review"`
	Implementation int `json:"implementation"`
}

// GuidelinesHistory tracks versions of guidelines for organizations, projects, and users
type GuidelinesHistory struct {
	ID             string    `json:"id" gorm:"primaryKey;type:varchar(255)"`
	OrganizationID string    `json:"organization_id,omitempty" gorm:"type:varchar(255);index"` // Set for org-level guidelines
	ProjectID      string    `json:"project_id,omitempty" gorm:"type:varchar(255);index"`      // Set for project-level guidelines
	UserID         string    `json:"user_id,omitempty" gorm:"type:varchar(255);index"`         // Set for user-level (personal workspace) guidelines
	Version        int       `json:"version"`
	Guidelines     string    `json:"guidelines" gorm:"type:text"`
	UpdatedBy      string    `json:"updated_by" gorm:"type:varchar(255)"` // User ID
	UpdatedByName  string    `json:"updated_by_name" gorm:"-"`            // User display name (not persisted, populated at query time)
	UpdatedByEmail string    `json:"updated_by_email" gorm:"-"`           // User email (not persisted, populated at query time)
	UpdatedAt      time.Time `json:"updated_at"`
	ChangeNote     string    `json:"change_note,omitempty" gorm:"type:text"` // Optional description of what changed
}
