package types

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Task represents a high-level work unit that can spawn multiple sessions
type Task struct {
	ID          string     `json:"id" gorm:"primaryKey;size:255"`
	Name        string     `json:"name" gorm:"not null;size:255"`
	Description string     `json:"description" gorm:"type:text"`
	TaskType    TaskType   `json:"task_type" gorm:"not null;size:50;index"`
	Status      TaskStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
	Priority    int        `json:"priority" gorm:"default:0;index"` // Lower = higher priority

	// Ownership
	UserID         string `json:"user_id" gorm:"not null;size:255;index"`
	AppID          string `json:"app_id" gorm:"size:255;index"`
	OrganizationID string `json:"organization_id" gorm:"size:255;index"`

	// Configuration
	Config datatypes.JSON `json:"config" gorm:"type:jsonb"`

	// State tracking
	CreatedAt   time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relationships (loaded via joins, not stored)
	WorkSessions []WorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// WorkSession represents an individual work unit within a task
// Maps 1:1 to a Helix Session
type WorkSession struct {
	ID             string `json:"id" gorm:"primaryKey;size:255"`
	TaskID         string `json:"task_id" gorm:"not null;size:255;index"`
	HelixSessionID string `json:"helix_session_id" gorm:"not null;size:255;uniqueIndex"` // 1:1 mapping

	// Work session details
	Name        string     `json:"name,omitempty" gorm:"size:255"`
	Description string     `json:"description,omitempty" gorm:"type:text"`
	AgentType   AgentType  `json:"agent_type" gorm:"not null;size:50;index"`
	Status      WorkStatus `json:"status" gorm:"not null;size:50;default:pending;index"`

	// Relationships for spawning/branching
	ParentWorkSessionID string `json:"parent_work_session_id,omitempty" gorm:"size:255;index"`
	SpawnedBySessionID  string `json:"spawned_by_session_id,omitempty" gorm:"size:255;index"`

	// Configuration
	AgentConfig       datatypes.JSON `json:"agent_config,omitempty" gorm:"type:jsonb"`
	EnvironmentConfig datatypes.JSON `json:"environment_config,omitempty" gorm:"type:jsonb"`

	// State tracking
	CreatedAt   time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UpdatedAt   time.Time  `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relationships (loaded via joins, not stored)
	Task              *Task             `json:"task,omitempty" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	HelixSession      *Session          `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ParentWorkSession *WorkSession      `json:"parent_work_session,omitempty" gorm:"foreignKey:ParentWorkSessionID"`
	SpawnedBySession  *WorkSession      `json:"spawned_by_session,omitempty" gorm:"foreignKey:SpawnedBySessionID"`
	ZedThreadMapping  *ZedThreadMapping `json:"zed_thread_mapping,omitempty" gorm:"foreignKey:WorkSessionID"`
}

// ZedThreadMapping maps Zed threads to work sessions (only for agent_type = 'zed_agent')
type ZedThreadMapping struct {
	ID            string `json:"id" gorm:"primaryKey;size:255"`
	WorkSessionID string `json:"work_session_id" gorm:"not null;size:255;uniqueIndex"` // 1:1 mapping
	ZedSessionID  string `json:"zed_session_id" gorm:"not null;size:255;index"`
	ZedThreadID   string `json:"zed_thread_id" gorm:"not null;size:255;index"`

	// Zed-specific configuration
	ProjectPath     string         `json:"project_path,omitempty" gorm:"size:500"`
	WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`

	// Status tracking
	Status         ZedStatus  `json:"status" gorm:"not null;size:50;default:pending;index"`
	LastActivityAt *time.Time `json:"last_activity_at,omitempty"`

	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Relationships (loaded via joins, not stored)
	WorkSession *WorkSession `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// Add unique constraint for zed_session_id + zed_thread_id combination
func (ZedThreadMapping) TableName() string {
	return "zed_thread_mappings"
}

// Enums

type TaskType string

const (
	TaskTypeInteractive   TaskType = "interactive"    // Long-running, user-driven (e.g., "User does coding")
	TaskTypeBatch         TaskType = "batch"          // Specific deliverable (e.g., "Implement feature X")
	TaskTypeCodingSession TaskType = "coding_session" // Focused session (e.g., "Debug performance issue")
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type AgentType string

const (
	AgentTypeHelixBasic  AgentType = "helix_basic"  // Basic Helix agent
	AgentTypeHelixAgent  AgentType = "helix_agent"  // Standard Helix agent with skills
	AgentTypeZedExternal AgentType = "zed_external" // Zed-integrated agent
)

// CodeAgentRuntime specifies which code agent runtime to use inside Zed.
// This determines how the LLM is configured within the Zed editor.
type CodeAgentRuntime string

const (
	// CodeAgentRuntimeZedAgent uses Zed's built-in agent panel.
	// The LLM is configured via Zed's settings (agent.default_model) and env vars
	// like ANTHROPIC_API_KEY. Works best with Anthropic and OpenAI models.
	CodeAgentRuntimeZedAgent CodeAgentRuntime = "zed_agent"

	// CodeAgentRuntimeQwenCode uses the qwen code agent as a custom agent_server.
	// The LLM is configured via OPENAI_BASE_URL, OPENAI_API_KEY, and OPENAI_MODEL
	// env vars passed to the qwen command. Works with any OpenAI-compatible API.
	CodeAgentRuntimeQwenCode CodeAgentRuntime = "qwen_code"
)

// Helper functions for agent type checking with backward compatibility

// MigrateAgentMode converts the old boolean agent_mode to the new AgentType enum
// This provides backward compatibility for existing configs
func (a *AssistantConfig) MigrateAgentMode() {
	// Only migrate if AgentType is not already set
	if a.AgentType == "" {
		if a.AgentMode {
			a.AgentType = AgentTypeHelixAgent
		} else {
			a.AgentType = AgentTypeHelixBasic
		}
	}
}

// IsAgentMode checks if an assistant is in any agent mode (backward compatible)
func (a *AssistantConfig) IsAgentMode() bool {
	// Ensure migration has happened
	a.MigrateAgentMode()
	return a.AgentType != AgentTypeHelixBasic
}

// GetAgentType returns the agent type, with fallback to boolean AgentMode
func (a *AssistantConfig) GetAgentType() AgentType {
	// Ensure migration has happened
	a.MigrateAgentMode()
	return a.AgentType
}

// IsAgentType checks if the assistant is of a specific agent type
func (a *AssistantConfig) IsAgentType(agentType AgentType) bool {
	return a.GetAgentType() == agentType
}

// GetDefaultAgentType returns the default agent type for an app
func (a *AppHelixConfig) GetDefaultAgentType() AgentType {
	if a.DefaultAgentType != "" {
		return a.DefaultAgentType
	}
	// If no default set, check if any assistant is in agent mode
	for _, assistant := range a.Assistants {
		if assistant.IsAgentMode() {
			return assistant.GetAgentType()
		}
	}
	return AgentTypeHelixBasic
}

// MigrateAgentTypes migrates all assistants in an app config from boolean to enum
func (a *AppHelixConfig) MigrateAgentTypes() {
	for i := range a.Assistants {
		a.Assistants[i].MigrateAgentMode()
	}

	// Set default agent type if not already set
	if a.DefaultAgentType == "" {
		a.DefaultAgentType = a.GetDefaultAgentType()
	}
}

type WorkStatus string

const (
	WorkStatusPending   WorkStatus = "pending"
	WorkStatusActive    WorkStatus = "active"
	WorkStatusCompleted WorkStatus = "completed"
	WorkStatusFailed    WorkStatus = "failed"
	WorkStatusCancelled WorkStatus = "cancelled"
)

type ZedStatus string

const (
	ZedStatusPending      ZedStatus = "pending"
	ZedStatusActive       ZedStatus = "active"
	ZedStatusDisconnected ZedStatus = "disconnected"
	ZedStatusCompleted    ZedStatus = "completed"
)

// Request types for API

type TaskCreateRequest struct {
	Name        string                 `json:"name" validate:"required,max=255"`
	Description string                 `json:"description,omitempty"`
	TaskType    TaskType               `json:"task_type" validate:"required,oneof=interactive batch coding_session"`
	Priority    int                    `json:"priority,omitempty"`
	AppID       string                 `json:"app_id,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type TaskUpdateRequest struct {
	Name        string                 `json:"name,omitempty" validate:"omitempty,max=255"`
	Description string                 `json:"description,omitempty"`
	Status      TaskStatus             `json:"status,omitempty" validate:"omitempty,oneof=pending active completed failed cancelled"`
	Priority    int                    `json:"priority,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type WorkSessionCreateRequest struct {
	TaskID              string                 `json:"task_id" validate:"required"`
	Name                string                 `json:"name,omitempty" validate:"omitempty,max=255"`
	Description         string                 `json:"description,omitempty"`
	AgentType           AgentType              `json:"agent_type" validate:"required,oneof=simple helix_agent zed_agent"`
	ParentWorkSessionID string                 `json:"parent_work_session_id,omitempty"`
	AgentConfig         map[string]interface{} `json:"agent_config,omitempty"`
	EnvironmentConfig   map[string]interface{} `json:"environment_config,omitempty"`
}

type WorkSessionUpdateRequest struct {
	Name        string                 `json:"name,omitempty" validate:"omitempty,max=255"`
	Description string                 `json:"description,omitempty"`
	Status      WorkStatus             `json:"status,omitempty" validate:"omitempty,oneof=pending active completed failed cancelled"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type ZedThreadCreateRequest struct {
	WorkSessionID   string                 `json:"work_session_id" validate:"required"`
	ProjectPath     string                 `json:"project_path,omitempty"`
	WorkspaceConfig map[string]interface{} `json:"workspace_config,omitempty"`
}

type ZedThreadUpdateRequest struct {
	Status          ZedStatus              `json:"status,omitempty" validate:"omitempty,oneof=pending active disconnected completed"`
	ProjectPath     string                 `json:"project_path,omitempty"`
	WorkspaceConfig map[string]interface{} `json:"workspace_config,omitempty"`
}

// Response types

type TaskListResponse struct {
	Tasks []Task `json:"tasks"`
	Total int    `json:"total"`
}

type WorkSessionListResponse struct {
	WorkSessions []WorkSession `json:"work_sessions"`
	Total        int           `json:"total"`
}

type TaskOverviewResponse struct {
	Task             Task          `json:"task"`
	WorkSessionCount int           `json:"work_session_count"`
	ActiveSessions   int           `json:"active_sessions"`
	ZedThreadCount   int           `json:"zed_thread_count"`
	LastActivity     *time.Time    `json:"last_activity,omitempty"`
	WorkSessions     []WorkSession `json:"work_sessions,omitempty"`
}

type WorkSessionDetailResponse struct {
	WorkSession      WorkSession       `json:"work_session"`
	Task             Task              `json:"task"`
	HelixSession     *Session          `json:"helix_session,omitempty"`
	ZedThreadMapping *ZedThreadMapping `json:"zed_thread_mapping,omitempty"`
}

// GORM Hooks for validation and consistency

func (t *Task) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = GenerateTaskID()
	}
	return nil
}

func (w *WorkSession) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		w.ID = GenerateWorkSessionID()
	}
	return nil
}

func (z *ZedThreadMapping) BeforeCreate(tx *gorm.DB) error {
	if z.ID == "" {
		z.ID = GenerateZedMappingID()
	}
	return nil
}

// Helper functions for ID generation (these would be implemented elsewhere)
// These are just placeholders showing the interface

func GenerateTaskID() string {
	// Implementation would generate a unique task ID
	return "task_" + uuid.New().String()
}

func GenerateWorkSessionID() string {
	// Implementation would generate a unique work session ID
	return "ws_" + uuid.New().String()
}

func GenerateZedMappingID() string {
	// Implementation would generate a unique zed mapping ID
	return "zm_" + uuid.New().String()
}
