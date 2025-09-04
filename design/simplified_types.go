package types

import (
	"time"

	"gorm.io/datatypes"
)

// Task represents a high-level work unit that can spawn multiple sessions
type Task struct {
	ID          string     `json:"id" gorm:"primaryKey"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	TaskType    TaskType   `json:"task_type"`
	Status      TaskStatus `json:"status" gorm:"default:pending"`
	Priority    int        `json:"priority" gorm:"default:0"` // Lower = higher priority

	// Ownership
	UserID         string `json:"user_id" gorm:"index"`
	AppID          string `json:"app_id"`
	OrganizationID string `json:"organization_id"`

	// Configuration
	Config datatypes.JSON `json:"config" gorm:"type:jsonb"`

	// State tracking
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relationships (loaded via joins)
	WorkSessions []WorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:TaskID"`
}

// WorkSession represents an individual work unit within a task
// Maps 1:1 to a Helix Session
type WorkSession struct {
	ID             string `json:"id" gorm:"primaryKey"`
	TaskID         string `json:"task_id" gorm:"index"`
	HelixSessionID string `json:"helix_session_id" gorm:"uniqueIndex"` // 1:1 mapping

	// Work session details
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	AgentType   AgentType         `json:"agent_type"`
	Status      WorkSessionStatus `json:"status" gorm:"default:pending"`

	// Relationships for spawning/branching
	ParentWorkSessionID string `json:"parent_work_session_id,omitempty"`
	SpawnedBySessionID  string `json:"spawned_by_session_id,omitempty"`

	// Configuration
	AgentConfig       datatypes.JSON `json:"agent_config,omitempty" gorm:"type:jsonb"`
	EnvironmentConfig datatypes.JSON `json:"environment_config,omitempty" gorm:"type:jsonb"`

	// State tracking
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relationships
	Task              *Task             `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	HelixSession      *Session          `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID"`
	ParentWorkSession *WorkSession      `json:"parent_work_session,omitempty" gorm:"foreignKey:ParentWorkSessionID"`
	SpawnedBySession  *WorkSession      `json:"spawned_by_session,omitempty" gorm:"foreignKey:SpawnedBySessionID"`
	ZedThreadMapping  *ZedThreadMapping `json:"zed_thread_mapping,omitempty" gorm:"foreignKey:WorkSessionID"`
}

// ZedInstanceMapping maps Zed agent instances to tasks (one instance per task)
type ZedInstanceMapping struct {
	ID              string            `json:"id" gorm:"primaryKey;size:255"`
	TaskID          string            `json:"task_id" gorm:"not null;size:255;uniqueIndex"` // One instance per task
	ZedInstanceID   string            `json:"zed_instance_id" gorm:"not null;size:255;index"`
	ProjectPath     string            `json:"project_path,omitempty" gorm:"size:500"`
	WorkspaceConfig datatypes.JSON    `json:"workspace_config,omitempty" gorm:"type:jsonb"`
	Status          ZedInstanceStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
	CreatedAt       time.Time         `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	LastActivityAt  *time.Time        `json:"last_activity_at,omitempty"`

	// Relationships
	Task           *Task              `json:"task,omitempty" gorm:"foreignKey:TaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ThreadMappings []ZedThreadMapping `json:"thread_mappings,omitempty" gorm:"foreignKey:ZedInstanceID"`
}

// ZedThreadMapping maps individual work sessions to threads within a Zed instance
type ZedThreadMapping struct {
	ID            string `json:"id" gorm:"primaryKey;size:255"`
	WorkSessionID string `json:"work_session_id" gorm:"not null;size:255;uniqueIndex"` // 1:1 mapping
	ZedInstanceID string `json:"zed_instance_id" gorm:"not null;size:255;index"`
	ZedThreadID   string `json:"zed_thread_id" gorm:"not null;size:255;index"`

	// Thread-specific configuration
	ThreadConfig   datatypes.JSON  `json:"thread_config,omitempty" gorm:"type:jsonb"`
	Status         ZedThreadStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
	LastActivityAt *time.Time      `json:"last_activity_at,omitempty"`

	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Relationships
	WorkSession *WorkSession        `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	ZedInstance *ZedInstanceMapping `json:"zed_instance,omitempty" gorm:"foreignKey:ZedInstanceID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
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
	AgentTypeSimple     AgentType = "simple"      // Basic Helix agent
	AgentTypeHelixAgent AgentType = "helix_agent" // Standard Helix agent with skills
	AgentTypeZedAgent   AgentType = "zed_agent"   // Zed-integrated agent
)

type WorkSessionStatus string

const (
	WorkSessionStatusPending   WorkSessionStatus = "pending"
	WorkSessionStatusActive    WorkSessionStatus = "active"
	WorkSessionStatusCompleted WorkSessionStatus = "completed"
	WorkSessionStatusFailed    WorkSessionStatus = "failed"
	WorkSessionStatusCancelled WorkSessionStatus = "cancelled"
)

type ZedInstanceStatus string

const (
	ZedInstanceStatusPending      ZedInstanceStatus = "pending"
	ZedInstanceStatusActive       ZedInstanceStatus = "active"
	ZedInstanceStatusDisconnected ZedInstanceStatus = "disconnected"
	ZedInstanceStatusCompleted    ZedInstanceStatus = "completed"
	ZedInstanceStatusFailed       ZedInstanceStatus = "failed"
)

type ZedThreadStatus string

const (
	ZedThreadStatusPending      ZedThreadStatus = "pending"
	ZedThreadStatusActive       ZedThreadStatus = "active"
	ZedThreadStatusDisconnected ZedThreadStatus = "disconnected"
	ZedThreadStatusCompleted    ZedThreadStatus = "completed"
)

// Request types for API

type TaskCreateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	TaskType    TaskType               `json:"task_type"`
	Priority    int                    `json:"priority,omitempty"`
	AppID       string                 `json:"app_id,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type TaskUpdateRequest struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Status      TaskStatus             `json:"status,omitempty"`
	Priority    int                    `json:"priority,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type WorkSessionCreateRequest struct {
	TaskID              string                 `json:"task_id"`
	Name                string                 `json:"name,omitempty"`
	Description         string                 `json:"description,omitempty"`
	AgentType           AgentType              `json:"agent_type"`
	ParentWorkSessionID string                 `json:"parent_work_session_id,omitempty"`
	AgentConfig         map[string]interface{} `json:"agent_config,omitempty"`
	EnvironmentConfig   map[string]interface{} `json:"environment_config,omitempty"`
}

type WorkSessionUpdateRequest struct {
	Name        string                 `json:"name,omitempty"`
	Description string                 `json:"description,omitempty"`
	Status      WorkSessionStatus      `json:"status,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

type ZedInstanceCreateRequest struct {
	TaskID          string                 `json:"task_id" validate:"required"`
	ProjectPath     string                 `json:"project_path,omitempty"`
	WorkspaceConfig map[string]interface{} `json:"workspace_config,omitempty"`
}

type ZedThreadCreateRequest struct {
	WorkSessionID string                 `json:"work_session_id" validate:"required"`
	ThreadConfig  map[string]interface{} `json:"thread_config,omitempty"`
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
	Task             Task                `json:"task"`
	WorkSessionCount int                 `json:"work_session_count"`
	ActiveSessions   int                 `json:"active_sessions"`
	ZedThreadCount   int                 `json:"zed_thread_count"`
	ZedInstance      *ZedInstanceMapping `json:"zed_instance,omitempty"`
	LastActivity     *time.Time          `json:"last_activity,omitempty"`
	WorkSessions     []WorkSession       `json:"work_sessions,omitempty"`
}

// Extensions to existing Session type
// These would be added to the existing Session struct:
// TaskID         string `json:"task_id,omitempty" gorm:"index"`
// WorkSessionID  string `json:"work_session_id,omitempty" gorm:"index"`

// Future: TaskContext for shared state across work sessions
// Will be implemented in a later phase
// type TaskContext struct {
//     ID          string `json:"id" gorm:"primaryKey"`
//     TaskID      string `json:"task_id" gorm:"index"`
//     ContextType string `json:"context_type"` // 'file_state', 'decision', 'shared_memory'
//     ContextKey  string `json:"context_key"`
//     ContextData datatypes.JSON `json:"context_data" gorm:"type:jsonb"`
//     CreatedAt   time.Time `json:"created_at"`
//     UpdatedAt   time.Time `json:"updated_at"`
// }
