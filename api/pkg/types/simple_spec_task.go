package types

import (
	"time"

	"gorm.io/datatypes"
)

// SpecTask represents a task following Kiro's actual spec-driven approach
// Simple, human-readable artifacts rather than complex nested structures
type SpecTask struct {
	ID          string `json:"id" gorm:"primaryKey"`
	ProjectID   string `json:"project_id" gorm:"index"`
	Name        string `json:"name"`
	Description string `json:"description" gorm:"type:text"`
	Type        string `json:"type"`     // "feature", "bug", "refactor"
	Priority    string `json:"priority"` // "low", "medium", "high", "critical"
	Status      string `json:"status"`   // Spec-driven workflow statuses - see constants below

	// Kiro's actual approach: simple, human-readable artifacts
	OriginalPrompt     string `json:"original_prompt" gorm:"type:text"`     // The user's original request
	RequirementsSpec   string `json:"requirements_spec" gorm:"type:text"`   // User stories + EARS acceptance criteria (markdown)
	TechnicalDesign    string `json:"technical_design" gorm:"type:text"`    // Design document (markdown)
	ImplementationPlan string `json:"implementation_plan" gorm:"type:text"` // Discrete tasks breakdown (markdown)

	// NEW: Single Helix Agent for entire workflow (App type in code)
	HelixAppID string `json:"helix_app_id,omitempty" gorm:"size:255;index"`

	// Git repository attachments: REMOVED - now inherited from parent Project
	// Repos are managed at the project level. Access via project.DefaultRepoID and GetProjectRepositories(project_id)

	// Session tracking (single Helix session for entire workflow - planning + implementation)
	// The same external agent/session is reused throughout the entire SpecTask lifecycle
	PlanningSessionID string `json:"planning_session_id,omitempty" gorm:"size:255;index"`

	// External agent tracking (single agent per SpecTask, spans entire workflow)
	ExternalAgentID string `json:"external_agent_id,omitempty" gorm:"size:255;index"`

	// Git tracking
	BranchName string `json:"branch_name,omitempty"`

	// Multi-session support
	ZedInstanceID   string         `json:"zed_instance_id,omitempty" gorm:"size:255;index"`
	ProjectPath     string         `json:"project_path,omitempty" gorm:"size:500"`
	WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`

	// Approval tracking
	SpecApprovedBy    string     `json:"spec_approved_by,omitempty"` // User who approved specs
	SpecApprovedAt    *time.Time `json:"spec_approved_at,omitempty"`
	SpecRevisionCount int        `json:"spec_revision_count"`            // Number of spec revisions requested
	YoloMode          bool       `json:"yolo_mode" gorm:"default:false"` // Skip human review, auto-approve specs

	// Implementation tracking
	ImplementationApprovedBy string     `json:"implementation_approved_by,omitempty"` // User who approved implementation
	ImplementationApprovedAt *time.Time `json:"implementation_approved_at,omitempty"`

	// Git tracking
	LastPushCommitHash string     `json:"last_push_commit_hash,omitempty"`     // Last commit hash pushed to feature branch
	LastPushAt         *time.Time `json:"last_push_at,omitempty"`              // When branch was last pushed
	DesignDocsPushedAt *time.Time `json:"design_docs_pushed_at,omitempty"`     // When design docs were pushed to helix-specs branch
	MergedToMain       bool       `json:"merged_to_main" gorm:"default:false"` // Whether branch was merged to main
	MergedAt           *time.Time `json:"merged_at,omitempty"`                 // When merge happened
	MergeCommitHash    string     `json:"merge_commit_hash,omitempty"`         // Merge commit hash

	// Simple tracking
	EstimatedHours int        `json:"estimated_hours,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`

	// Metadata
	CreatedBy string                 `json:"created_by"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
	Archived  bool                   `json:"archived" gorm:"default:false;index"` // Archive to hide from main view
	Labels    []string               `json:"labels" gorm:"type:jsonb;serializer:json"`
	Metadata  map[string]interface{} `json:"metadata,omitempty" gorm:"type:jsonb;serializer:json"`

	// Relationships (loaded via joins, not stored in database)
	// NOTE: Use GORM preloading to load these when needed:
	//   db.Preload("WorkSessions").Preload("ZedThreads").Find(&specTask)
	// swaggerignore prevents circular reference in swagger generation
	WorkSessions []SpecTaskWorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:SpecTaskID" swaggerignore:"true"`
	ZedThreads   []SpecTaskZedThread   `json:"zed_threads,omitempty" gorm:"foreignKey:SpecTaskID" swaggerignore:"true"`
}

// SampleSpecProject - simplified sample projects with proper spec-driven tasks
type SampleSpecProject struct {
	ID            string             `json:"id"`
	Name          string             `json:"name"`
	Description   string             `json:"description"`
	GitHubRepo    string             `json:"github_repo"`
	DefaultBranch string             `json:"default_branch"`
	Technologies  []string           `json:"technologies"`
	TaskPrompts   []SampleTaskPrompt `json:"task_prompts"` // Just prompts - specs generated dynamically
	ReadmeURL     string             `json:"readme_url"`
	DemoURL       string             `json:"demo_url,omitempty"`
	Difficulty    string             `json:"difficulty"`
	Category      string             `json:"category"`
}

// SampleTaskPrompt - just the user prompt, following Kiro's approach
type SampleTaskPrompt struct {
	Prompt      string   `json:"prompt"` // Natural language request like Kiro expects
	Priority    string   `json:"priority"`
	Labels      []string `json:"labels"`
	Context     string   `json:"context"`     // Additional context about the codebase
	Constraints string   `json:"constraints"` // Any specific constraints
}

// SpecGeneration represents the AI-generated specs from a prompt
type SpecGeneration struct {
	TaskID             string    `json:"task_id"`
	RequirementsSpec   string    `json:"requirements_spec"`   // Generated user stories + EARS criteria
	TechnicalDesign    string    `json:"technical_design"`    // Generated design doc
	ImplementationPlan string    `json:"implementation_plan"` // Generated task breakdown
	GeneratedAt        time.Time `json:"generated_at"`
	ModelUsed          string    `json:"model_used"`
	TokensUsed         int       `json:"tokens_used"`
}

// SpecTaskFilters for filtering spec tasks in queries
type SpecTaskFilters struct {
	ProjectID       string `json:"project_id,omitempty"`
	Status          string `json:"status,omitempty"`
	UserID          string `json:"user_id,omitempty"`
	Type            string `json:"type,omitempty"`
	Priority        string `json:"priority,omitempty"`
	Limit           int    `json:"limit,omitempty"`
	Offset          int    `json:"offset,omitempty"`
	IncludeArchived bool   `json:"include_archived,omitempty"` // If true, include both archived and non-archived
	ArchivedOnly    bool   `json:"archived_only,omitempty"`    // If true, show only archived tasks
}

// SpecTaskUpdateRequest represents a request to update a SpecTask
type SpecTaskUpdateRequest struct {
	Status      string `json:"status,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
	YoloMode    *bool  `json:"yolo_mode,omitempty"` // Pointer to allow explicit false
}

// Two-phase workflow status constants
const (
	// Phase 1: Specification Generation (Helix Agent)
	TaskStatusBacklog        = "backlog"         // Initial state, waiting for spec generation
	TaskStatusSpecGeneration = "spec_generation" // Helix agent generating specs
	TaskStatusSpecReview     = "spec_review"     // Human reviewing generated specs
	TaskStatusSpecRevision   = "spec_revision"   // Human requested spec changes
	TaskStatusSpecApproved   = "spec_approved"   // Specs approved, ready for implementation

	// Phase 2: Implementation (Zed Agent)
	TaskStatusImplementationQueued = "implementation_queued" // Waiting for Zed agent pickup
	TaskStatusImplementation       = "implementation"        // Zed agent coding
	TaskStatusImplementationReview = "implementation_review" // Code review (PR created)
	TaskStatusDone                 = "done"                  // Task completed

	// Error states
	TaskStatusSpecFailed           = "spec_failed"           // Spec generation failed
	TaskStatusImplementationFailed = "implementation_failed" // Implementation failed
)

// Agent specialization types
const (
	AgentTypeSpecGeneration = "spec_generation" // Helix agents for planning/specs
	AgentTypeImplementation = "implementation"  // Zed agents for coding
)

// SpecApprovalRequest represents a request for human spec approval
type SpecApprovalRequest struct {
	TaskID             string    `json:"task_id"`
	RequirementsSpec   string    `json:"requirements_spec"`
	TechnicalDesign    string    `json:"technical_design"`
	ImplementationPlan string    `json:"implementation_plan"`
	ReviewerID         string    `json:"reviewer_id"`
	RequestedAt        time.Time `json:"requested_at"`
	Comments           string    `json:"comments,omitempty"`
}

// SpecApprovalResponse represents the human response to spec review
type SpecApprovalResponse struct {
	TaskID     string    `json:"task_id"`
	Approved   bool      `json:"approved"`
	Comments   string    `json:"comments,omitempty"`
	Changes    []string  `json:"changes,omitempty"` // Specific requested changes
	ApprovedBy string    `json:"approved_by"`
	ApprovedAt time.Time `json:"approved_at"`
}

// SpecTaskExternalAgent represents the external agent (Wolf container) for a SpecTask
// Single agent per SpecTask that spans multiple Helix sessions via Zed threads
type SpecTaskExternalAgent struct {
	ID              string    `json:"id" gorm:"primaryKey;size:255"`                       // zed-spectask-{spectask_id}
	SpecTaskID      string    `json:"spec_task_id" gorm:"not null;size:255;index"`         // Parent SpecTask
	WolfAppID       string    `json:"wolf_app_id" gorm:"size:255"`                         // Wolf app managing this agent
	WorkspaceDir    string    `json:"workspace_dir" gorm:"size:500"`                       // /workspaces/spectasks/{id}/work/
	HelixSessionIDs []string  `json:"helix_session_ids" gorm:"type:jsonb;serializer:json"` // All sessions using this agent
	ZedThreadIDs    []string  `json:"zed_thread_ids" gorm:"type:jsonb;serializer:json"`    // Zed threads (1:1 with sessions)
	Status          string    `json:"status" gorm:"size:50;default:creating;index"`        // creating, running, terminated
	Created         time.Time `json:"created" gorm:"not null;default:CURRENT_TIMESTAMP"`
	LastActivity    time.Time `json:"last_activity" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UserID          string    `json:"user_id" gorm:"size:255;index"`
}

// ExternalAgentActivity tracks activity for idle detection (per-agent, not per-session)
type ExternalAgentActivity struct {
	ExternalAgentID string    `json:"external_agent_id" gorm:"primaryKey;size:255"` // e.g., "zed-spectask-abc123"
	SpecTaskID      string    `json:"spec_task_id" gorm:"not null;size:255;index"`  // Parent SpecTask
	LastInteraction time.Time `json:"last_interaction" gorm:"not null;index"`
	AgentType       string    `json:"agent_type" gorm:"size:50"`     // "spectask", "pde", "adhoc"
	WolfAppID       string    `json:"wolf_app_id" gorm:"size:255"`   // Wolf app ID for termination
	WolfLobbyID     string    `json:"wolf_lobby_id" gorm:"size:255"` // Wolf lobby ID for cleanup even after session deleted
	WolfLobbyPIN    string    `json:"wolf_lobby_pin" gorm:"size:4"`  // Wolf lobby PIN for cleanup
	WorkspaceDir    string    `json:"workspace_dir" gorm:"size:500"` // Persistent workspace path
	UserID          string    `json:"user_id" gorm:"size:255;index"`
}

// Table names
func (SpecTaskExternalAgent) TableName() string {
	return "spec_task_external_agents"
}

func (ExternalAgentActivity) TableName() string {
	return "external_agent_activity"
}

type SpecTaskArchiveRequest struct {
	Archived bool `json:"archived"`
}
