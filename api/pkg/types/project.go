package types

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ProjectSpec is the declarative specification for a project YAML (helix apply -f / kubectl apply -f).
type ProjectSpec struct {
	Name                  string                  `json:"name,omitempty" yaml:"name,omitempty"`
	Description           string                  `json:"description,omitempty" yaml:"description,omitempty"`
	Technologies          []string                `json:"technologies,omitempty" yaml:"technologies,omitempty"`
	Guidelines            string                  `json:"guidelines,omitempty" yaml:"guidelines,omitempty"`
	Repository            *ProjectRepositorySpec  `json:"repository,omitempty" yaml:"repository,omitempty"`     // Singular shorthand
	Repositories          []ProjectRepositorySpec `json:"repositories,omitempty" yaml:"repositories,omitempty"` // Multi-repo list
	Startup               *ProjectStartup         `json:"startup,omitempty" yaml:"startup,omitempty"`
	Kanban                *ProjectKanban          `json:"kanban,omitempty" yaml:"kanban,omitempty"`
	Tasks                 []ProjectTaskSpec       `json:"tasks,omitempty" yaml:"tasks,omitempty"`
	Agent                 *ProjectAgentSpec       `json:"agent,omitempty" yaml:"agent,omitempty"`
	AutoStartBacklogTasks bool                    `json:"auto_start_backlog_tasks,omitempty" yaml:"auto_start_backlog_tasks,omitempty"`
}

// ValidateRepositories validates the repository configuration in the spec.
func (s *ProjectSpec) ValidateRepositories() error {
	if s.Repository != nil && len(s.Repositories) > 0 {
		return fmt.Errorf("cannot specify both 'repository' and 'repositories'")
	}
	resolved := s.ResolvedRepositories()
	if len(resolved) > 1 {
		primaryCount := 0
		for _, r := range resolved {
			if r.Primary {
				primaryCount++
			}
		}
		if primaryCount == 0 {
			return fmt.Errorf("exactly one repository must be designated primary")
		}
		if primaryCount > 1 {
			return fmt.Errorf("only one repository may be designated primary")
		}
	}
	return nil
}

// ResolvedRepositories normalises singular/plural into a single slice.
// If only one repository is present, primary is implied.
func (s *ProjectSpec) ResolvedRepositories() []ProjectRepositorySpec {
	var repos []ProjectRepositorySpec
	if s.Repository != nil {
		repo := *s.Repository
		repo.Primary = true
		repos = []ProjectRepositorySpec{repo}
	} else {
		repos = s.Repositories
	}
	if len(repos) == 1 {
		repos[0].Primary = true
	}
	return repos
}

// ProjectRepositorySpec describes a repository attachment in a project YAML.
type ProjectRepositorySpec struct {
	URL           string `json:"url" yaml:"url"`
	DefaultBranch string `json:"default_branch,omitempty" yaml:"default_branch,omitempty"`
	Primary       bool   `json:"primary,omitempty" yaml:"primary,omitempty"`
}

// ProjectStartup describes startup commands that run in the primary repository.
type ProjectStartup struct {
	// Script is the unified startup script content (preferred)
	Script string `json:"script,omitempty" yaml:"script,omitempty"`
	// Install and Start are deprecated - use Script instead
	// Kept for backward compatibility with existing YAML files
	Install string `json:"install,omitempty" yaml:"install,omitempty"`
	Start   string `json:"start,omitempty" yaml:"start,omitempty"`
}

// ProjectKanban holds Kanban board settings from a project YAML.
type ProjectKanban struct {
	WIPLimits *ProjectWIPLimits `json:"wip_limits,omitempty" yaml:"wip_limits,omitempty"`
}

// ProjectWIPLimits holds per-column WIP limit values.
type ProjectWIPLimits struct {
	Planning       int `json:"planning,omitempty" yaml:"planning,omitempty"`
	Implementation int `json:"implementation,omitempty" yaml:"implementation,omitempty"`
	Review         int `json:"review,omitempty" yaml:"review,omitempty"`
}

// ProjectTaskSpec is a task to seed onto the Kanban board when the YAML is applied.
// Intended for demos and project templates; omit in production YAMLs.
type ProjectTaskSpec struct {
	Title       string `json:"title" yaml:"title"`
	Description string `json:"description,omitempty" yaml:"description,omitempty"`
}

// ProjectAgentSpec is the simplified agent configuration in a project YAML.
// It is converted to a full Helix App when applyProject runs, and stored
// as the project's DefaultHelixAppID.
//
// Runtime selects the code agent inside the Zed desktop container.
// Defaults to "claude_code" when omitted (recommended — handles context compaction).
//   - "claude_code" — Claude Code CLI (default)
//   - "zed"         — Zed's built-in agent panel
//   - "qwen_code"   — Qwen Code CLI
//   - "gemini_cli"  — Gemini CLI
//   - "codex_cli"   — OpenAI Codex CLI
//
// Credentials selects how the agent authenticates with the LLM provider:
//   - "api_key"      — (default) routes through Helix LLM proxy using the user's API key
//   - "subscription" — uses OAuth credentials directly (e.g. a Claude subscription)
type ProjectAgentSpec struct {
	Name        string               `json:"name,omitempty" yaml:"name,omitempty"`
	Runtime     string               `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	Model       string               `json:"model,omitempty" yaml:"model,omitempty"`
	Provider    string               `json:"provider,omitempty" yaml:"provider,omitempty"`
	Credentials string               `json:"credentials,omitempty" yaml:"credentials,omitempty"`
	Tools       *ProjectAgentTools   `json:"tools,omitempty" yaml:"tools,omitempty"`
	Display     *ProjectAgentDisplay `json:"display,omitempty" yaml:"display,omitempty"`
}

// ProjectAgentTools lists the built-in tools to enable for the project agent.
type ProjectAgentTools struct {
	WebSearch  bool `json:"web_search,omitempty" yaml:"web_search,omitempty"`
	Browser    bool `json:"browser,omitempty" yaml:"browser,omitempty"`
	Calculator bool `json:"calculator,omitempty" yaml:"calculator,omitempty"`
}

// ProjectAgentDisplay configures the virtual desktop for the agent container.
type ProjectAgentDisplay struct {
	// Resolution preset: "1080p" (default), "4k", or "5k"
	Resolution string `json:"resolution,omitempty" yaml:"resolution,omitempty"`
	// Desktop environment: "ubuntu" (default GNOME) or "sway"
	DesktopType string `json:"desktop_type,omitempty" yaml:"desktop_type,omitempty"`
	// Display refresh rate in Hz (default 60)
	FPS int `json:"fps,omitempty" yaml:"fps,omitempty"`
}

// ProjectCRD is the top-level structure for a project YAML file.
type ProjectCRD struct {
	APIVersion string             `json:"apiVersion" yaml:"apiVersion"`
	Kind       string             `json:"kind" yaml:"kind"`
	Metadata   ProjectCRDMetadata `json:"metadata" yaml:"metadata"`
	Spec       ProjectSpec        `json:"spec" yaml:"spec"`
}

// ProjectCRDMetadata holds metadata for a project CRD.
type ProjectCRDMetadata struct {
	Name string `json:"name" yaml:"name"`
}

// ProjectApplyRequest is the request body for PUT /api/v1/projects/apply.
type ProjectApplyRequest struct {
	OrganizationID string      `json:"organization_id"`
	Name           string      `json:"name"`
	Spec           ProjectSpec `json:"spec"`
}

// ProjectApplyResponse is the response for PUT /api/v1/projects/apply.
type ProjectApplyResponse struct {
	ProjectID  string `json:"project_id"`
	AgentAppID string `json:"agent_app_id,omitempty"`
	Created    bool   `json:"created"` // true if created, false if updated
}

// Project represents a Helix project that can contain tasks and agent work
type Project struct {
	ID             string   `json:"id" gorm:"primaryKey"`
	Name           string   `json:"name" gorm:"index"` // Indexed for search prefix matching
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

	// Startup commands from declarative project YAML (persisted)
	StartupInstall string `json:"startup_install,omitempty"`
	StartupStart   string `json:"startup_start,omitempty"`

	// Automation settings
	AutoStartBacklogTasks bool `json:"auto_start_backlog_tasks"` // Automatically move backlog tasks to planning when capacity available

	// StartupScriptFromYAML indicates the startup script was set via project YAML
	// When true, the UI should show the script as read-only
	StartupScriptFromYAML bool `json:"startup_script_from_yaml" gorm:"default:false"`

	// Default agent for spec tasks in this project (App ID)
	// New spec tasks inherit this agent; can be overridden per-task
	DefaultHelixAppID string `json:"default_helix_app_id"`

	ProjectManagerHelixAppID string `json:"project_manager_helix_app_id"`

	PullRequestReviewerHelixAppID string `json:"pull_request_reviewer_helix_app_id"`
	PullRequestReviewsEnabled     bool   `json:"pull_request_reviews_enabled"`
	KoditEnabled                  bool   `json:"kodit_enabled" gorm:"default:true"`

	// Guidelines for AI agents - project-specific style guides, conventions, and instructions
	// Combined with organization guidelines when constructing prompts
	Guidelines          string    `json:"guidelines"`
	GuidelinesVersion   int       `json:"guidelines_version"`    // Incremented on each update
	GuidelinesUpdatedAt time.Time `json:"guidelines_updated_at"` // When guidelines were last updated
	GuidelinesUpdatedBy string    `json:"guidelines_updated_by"` // User ID who last updated guidelines

	// Project-level skills - these overlay on top of agent skills
	// Useful for project-specific tools like CI integration (e.g., drone-ci-mcp)
	Skills *AssistantSkills `json:"skills,omitempty" gorm:"type:jsonb;serializer:json"`

	// Auto-incrementing task number for human-readable directory names
	// Each SpecTask gets assigned the next number (install-cowsay_1, add-api_2, etc.)
	NextTaskNumber int `json:"next_task_number" gorm:"default:1"`

	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
	DeletedAt gorm.DeletedAt  `json:"deleted_at,omitempty" gorm:"index"` // Soft delete timestamp
	Metadata  ProjectMetadata `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`

	Stats ProjectStats `json:"stats,omitempty" gorm:"-"` // Computed
}

type ProjectStats struct {
	TotalTasks          int     `json:"total_tasks"`
	CompletedTasks      int     `json:"completed_tasks"`
	InProgressTasks     int     `json:"in_progress_tasks"`
	BacklogTasks        int     `json:"backlog_tasks"`
	PlanningTasks       int     `json:"planning_tasks"`
	PendingReviewTasks  int     `json:"pending_review_tasks"`
	ActiveAgentSessions int     `json:"active_agent_sessions"`
	AverageTaskTime     float64 `json:"average_task_completion_hours"`
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
	OrganizationID    string           `json:"organization_id"`
	Name              string           `json:"name"`
	Description       string           `json:"description"`
	GitHubRepoURL     string           `json:"github_repo_url,omitempty"`
	DefaultBranch     string           `json:"default_branch,omitempty"`
	Technologies      []string         `json:"technologies,omitempty"`
	DefaultRepoID     string           `json:"default_repo_id,omitempty"`
	StartupScript     string           `json:"startup_script,omitempty"`
	DefaultHelixAppID string           `json:"default_helix_app_id,omitempty"` // Default agent for spec tasks
	Guidelines        string           `json:"guidelines,omitempty"`           // Project-specific AI agent guidelines
	Skills            *AssistantSkills `json:"skills,omitempty"`               // Project-level skills
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
	KoditEnabled                  *bool            `json:"kodit_enabled,omitempty"`                      // Whether Kodit code intelligence is enabled
	Guidelines                    *string          `json:"guidelines,omitempty"`                         // Project-specific AI agent guidelines
	Skills                        *AssistantSkills `json:"skills,omitempty"`                             // Project-level skills
	Metadata                      *ProjectMetadata `json:"metadata,omitempty"`
}

// MoveProjectRequest represents a request to move a project to an organization
type MoveProjectRequest struct {
	OrganizationID string `json:"organization_id"`
}

// MoveProjectPreviewResponse represents the preview of moving a project to an organization
type MoveProjectPreviewResponse struct {
	Project      MoveProjectPreviewItem      `json:"project"`
	Repositories []MoveRepositoryPreviewItem `json:"repositories"`
	// Warnings about things that won't be moved automatically
	Warnings []string `json:"warnings,omitempty"`
}

// MoveProjectPreviewItem represents a project's naming conflict status
type MoveProjectPreviewItem struct {
	CurrentName string  `json:"current_name"`
	NewName     *string `json:"new_name"` // nil if no conflict
	HasConflict bool    `json:"has_conflict"`
}

// MoveRepositoryPreviewItem represents a repository's naming conflict status
type MoveRepositoryPreviewItem struct {
	ID               string                `json:"id"`
	CurrentName      string                `json:"current_name"`
	NewName          *string               `json:"new_name"` // nil if no conflict
	HasConflict      bool                  `json:"has_conflict"`
	AffectedProjects []AffectedProjectInfo `json:"affected_projects,omitempty"` // Other projects that will lose this repo
}

// AffectedProjectInfo represents a project that will be affected by moving a shared repo
type AffectedProjectInfo struct {
	ID   string `json:"id"`
	Name string `json:"name"`
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
	BoardSettings       *BoardSettings    `json:"board_settings,omitempty"`
	AutoWarmDockerCache bool              `json:"auto_warm_docker_cache,omitempty"`
	DockerCacheStatus   *DockerCacheState `json:"docker_cache_status,omitempty"`
}

// DockerCacheState tracks the current state of the golden Docker cache for a project,
// per sandbox. Each sandbox has its own cache state since golden caches are local
// to the sandbox's filesystem.
type DockerCacheState struct {
	Sandboxes map[string]*SandboxCacheState `json:"sandboxes,omitempty"`
}

// OverallStatus returns an aggregate status across all sandboxes:
// "building" if any sandbox is building, "ready" if all are ready,
// "failed" if any failed (and none building), "none" otherwise.
func (d *DockerCacheState) OverallStatus() string {
	if d == nil || len(d.Sandboxes) == 0 {
		return "none"
	}
	anyBuilding := false
	anyFailed := false
	allReady := true
	for _, s := range d.Sandboxes {
		switch s.Status {
		case "building":
			anyBuilding = true
			allReady = false
		case "failed":
			anyFailed = true
			allReady = false
		case "ready":
			// ok
		default:
			allReady = false
		}
	}
	if anyBuilding {
		return "building"
	}
	if allReady && len(d.Sandboxes) > 0 {
		return "ready"
	}
	if anyFailed {
		return "failed"
	}
	return "none"
}

// SandboxCacheState tracks the golden Docker cache state for a single sandbox.
type SandboxCacheState struct {
	Status         string     `json:"status"`
	SizeBytes      int64      `json:"size_bytes,omitempty"`
	LastBuildAt    *time.Time `json:"last_build_at,omitempty"`
	LastReadyAt    *time.Time `json:"last_ready_at,omitempty"`
	BuildSessionID string     `json:"build_session_id,omitempty"`
	Error          string     `json:"error,omitempty"`
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
