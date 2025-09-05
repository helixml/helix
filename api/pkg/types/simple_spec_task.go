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

	// Spec-driven agent assignment
	SpecAgent               string `json:"spec_agent,omitempty"`                // Helix agent for spec generation
	ImplementationAgent     string `json:"implementation_agent,omitempty"`      // Zed agent for coding
	SpecSessionID           string `json:"spec_session_id,omitempty"`           // Helix session for spec phase
	ImplementationSessionID string `json:"implementation_session_id,omitempty"` // Zed session for implementation
	BranchName              string `json:"branch_name,omitempty"`

	// Multi-session support
	ZedInstanceID   string         `json:"zed_instance_id,omitempty" gorm:"size:255;index"`
	ProjectPath     string         `json:"project_path,omitempty" gorm:"size:500"`
	WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`

	// Approval tracking
	SpecApprovedBy    string     `json:"spec_approved_by,omitempty"` // User who approved specs
	SpecApprovedAt    *time.Time `json:"spec_approved_at,omitempty"`
	SpecRevisionCount int        `json:"spec_revision_count"` // Number of spec revisions requested

	// Simple tracking
	EstimatedHours int        `json:"estimated_hours,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`

	// Metadata
	CreatedBy string         `json:"created_by"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	Labels    []string       `json:"labels" gorm:"-"`
	LabelsDB  datatypes.JSON `json:"-" gorm:"column:labels;type:jsonb"`
	Metadata  datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Relationships (loaded via joins, not stored)
	WorkSessions []SpecTaskWorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:SpecTaskID"`
	ZedThreads   []SpecTaskZedThread   `json:"zed_threads,omitempty" gorm:"foreignKey:SpecTaskID"`
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
	ProjectID string `json:"project_id,omitempty"`
	Status    string `json:"status,omitempty"`
	UserID    string `json:"user_id,omitempty"`
	Type      string `json:"type,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`
}

// SpecTaskUpdateRequest represents a request to update a SpecTask
type SpecTaskUpdateRequest struct {
	Status      string `json:"status,omitempty"`
	Priority    string `json:"priority,omitempty"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
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
