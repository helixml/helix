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
	AppID          string `json:"app_id" gorm:"index"`
	OrganizationID string `json:"organization_id"`

	// Configuration
	Config            datatypes.JSON `json:"config" gorm:"type:jsonb"`             // Task-specific configuration
	ConstraintsConfig datatypes.JSON `json:"constraints_config" gorm:"type:jsonb"` // Resource constraints, time limits, etc.

	// State tracking
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	DeadlineAt  *time.Time `json:"deadline_at,omitempty"`

	// Metadata
	Metadata datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`
	Labels   datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"` // Tags for filtering/organization

	// Relationships
	ParentTaskID string `json:"parent_task_id,omitempty"`
	ParentTask   *Task  `json:"parent_task,omitempty" gorm:"foreignKey:ParentTaskID"`

	// Associated entities (loaded via joins)
	TaskSession  *TaskSession  `json:"task_session,omitempty" gorm:"foreignKey:TaskID"`
	WorkSessions []WorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:TaskID"`
	Contexts     []TaskContext `json:"contexts,omitempty" gorm:"foreignKey:TaskID"`
}

// TaskSession represents the coordination/orchestration session for a task
type TaskSession struct {
	ID              string            `json:"id" gorm:"primaryKey"`
	TaskID          string            `json:"task_id" gorm:"uniqueIndex"`
	HelixSessionID  string            `json:"helix_session_id"`
	Status          TaskSessionStatus `json:"status" gorm:"default:active"`
	CoordinatorType string            `json:"coordinator_type" gorm:"default:helix_coordinator"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	Task         *Task    `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	HelixSession *Session `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID"`
}

// WorkSession represents an individual work unit within a task
type WorkSession struct {
	ID             string `json:"id" gorm:"primaryKey"`
	TaskID         string `json:"task_id" gorm:"index"`
	HelixSessionID string `json:"helix_session_id"`

	// Work session details
	Name        string            `json:"name,omitempty"`
	Description string            `json:"description,omitempty"`
	WorkType    WorkSessionType   `json:"work_type"`
	Status      WorkSessionStatus `json:"status" gorm:"default:pending"`

	// Relationships
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

	// Metadata
	Metadata datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Relationships
	Task              *Task               `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	HelixSession      *Session            `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID"`
	ParentWorkSession *WorkSession        `json:"parent_work_session,omitempty" gorm:"foreignKey:ParentWorkSessionID"`
	SpawnedBySession  *WorkSession        `json:"spawned_by_session,omitempty" gorm:"foreignKey:SpawnedBySessionID"`
	ZedThreadMapping  *ZedThreadMapping   `json:"zed_thread_mapping,omitempty" gorm:"foreignKey:WorkSessionID"`
	Dependencies      []SessionDependency `json:"dependencies,omitempty" gorm:"foreignKey:DependentSessionID"`
	DependentSessions []SessionDependency `json:"dependent_sessions,omitempty" gorm:"foreignKey:DependencySessionID"`
}

// ZedThreadMapping maps Zed threads to work sessions
type ZedThreadMapping struct {
	ID            string `json:"id" gorm:"primaryKey"`
	WorkSessionID string `json:"work_session_id"`
	ZedSessionID  string `json:"zed_session_id"`
	ZedThreadID   string `json:"zed_thread_id"`

	// Zed-specific configuration
	ProjectPath     string         `json:"project_path,omitempty"`
	WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`

	// Status tracking
	Status         ZedThreadStatus `json:"status" gorm:"default:pending"`
	LastActivityAt *time.Time      `json:"last_activity_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships
	WorkSession *WorkSession `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID"`
}

// TaskContext represents shared state across task sessions
type TaskContext struct {
	ID          string          `json:"id" gorm:"primaryKey"`
	TaskID      string          `json:"task_id" gorm:"index:idx_task_context_unique,priority:1"`
	ContextType TaskContextType `json:"context_type" gorm:"index:idx_task_context_unique,priority:2"`
	ContextKey  string          `json:"context_key" gorm:"index:idx_task_context_unique,priority:3"`
	ContextData datatypes.JSON  `json:"context_data" gorm:"type:jsonb"`

	// Metadata
	CreatedBySessionID string         `json:"created_by_session_id,omitempty"`
	AccessPermissions  datatypes.JSON `json:"access_permissions,omitempty" gorm:"type:jsonb"`

	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`

	// Relationships
	Task             *Task        `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	CreatedBySession *WorkSession `json:"created_by_session,omitempty" gorm:"foreignKey:CreatedBySessionID"`
}

// SessionDependency represents dependencies between work sessions
type SessionDependency struct {
	ID                  string                  `json:"id" gorm:"primaryKey"`
	TaskID              string                  `json:"task_id" gorm:"index"`
	DependentSessionID  string                  `json:"dependent_session_id" gorm:"index"`
	DependencySessionID string                  `json:"dependency_session_id" gorm:"index"`
	DependencyType      SessionDependencyType   `json:"dependency_type"`
	Status              SessionDependencyStatus `json:"status" gorm:"default:pending"`

	// Conditions for dependency satisfaction
	ConditionConfig datatypes.JSON `json:"condition_config,omitempty" gorm:"type:jsonb"`

	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	// Relationships
	Task              *Task        `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	DependentSession  *WorkSession `json:"dependent_session,omitempty" gorm:"foreignKey:DependentSessionID"`
	DependencySession *WorkSession `json:"dependency_session,omitempty" gorm:"foreignKey:DependencySessionID"`
}

// TaskExecutionLog represents events in task execution
type TaskExecutionLog struct {
	ID            string         `json:"id" gorm:"primaryKey"`
	TaskID        string         `json:"task_id" gorm:"index"`
	WorkSessionID string         `json:"work_session_id,omitempty" gorm:"index"`
	EventType     TaskEventType  `json:"event_type"`
	EventData     datatypes.JSON `json:"event_data" gorm:"type:jsonb"`

	CreatedAt time.Time `json:"created_at"`

	// Relationships
	Task        *Task        `json:"task,omitempty" gorm:"foreignKey:TaskID"`
	WorkSession *WorkSession `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID"`
}

// Enums and constants

type TaskType string

const (
	TaskTypeInteractive   TaskType = "interactive"
	TaskTypeBatch         TaskType = "batch"
	TaskTypeCodingSession TaskType = "coding_session"
)

type TaskStatus string

const (
	TaskStatusPending   TaskStatus = "pending"
	TaskStatusActive    TaskStatus = "active"
	TaskStatusPaused    TaskStatus = "paused"
	TaskStatusCompleted TaskStatus = "completed"
	TaskStatusFailed    TaskStatus = "failed"
	TaskStatusCancelled TaskStatus = "cancelled"
)

type TaskSessionStatus string

const (
	TaskSessionStatusActive    TaskSessionStatus = "active"
	TaskSessionStatusPaused    TaskSessionStatus = "paused"
	TaskSessionStatusCompleted TaskSessionStatus = "completed"
)

type WorkSessionType string

const (
	WorkSessionTypeZedThread   WorkSessionType = "zed_thread"
	WorkSessionTypeDirectHelix WorkSessionType = "direct_helix"
	WorkSessionTypeSubprocess  WorkSessionType = "subprocess"
)

type WorkSessionStatus string

const (
	WorkSessionStatusPending   WorkSessionStatus = "pending"
	WorkSessionStatusActive    WorkSessionStatus = "active"
	WorkSessionStatusCompleted WorkSessionStatus = "completed"
	WorkSessionStatusFailed    WorkSessionStatus = "failed"
	WorkSessionStatusCancelled WorkSessionStatus = "cancelled"
)

type ZedThreadStatus string

const (
	ZedThreadStatusPending      ZedThreadStatus = "pending"
	ZedThreadStatusActive       ZedThreadStatus = "active"
	ZedThreadStatusDisconnected ZedThreadStatus = "disconnected"
	ZedThreadStatusCompleted    ZedThreadStatus = "completed"
)

type TaskContextType string

const (
	TaskContextTypeFileState    TaskContextType = "file_state"
	TaskContextTypeDecision     TaskContextType = "decision"
	TaskContextTypeSharedMemory TaskContextType = "shared_memory"
	TaskContextTypeProjectState TaskContextType = "project_state"
)

type SessionDependencyType string

const (
	SessionDependencyTypeBlocks       SessionDependencyType = "blocks"
	SessionDependencyTypeWaitsFor     SessionDependencyType = "waits_for"
	SessionDependencyTypeMergesWith   SessionDependencyType = "merges_with"
	SessionDependencyTypeBranchesFrom SessionDependencyType = "branches_from"
)

type SessionDependencyStatus string

const (
	SessionDependencyStatusPending   SessionDependencyStatus = "pending"
	SessionDependencyStatusSatisfied SessionDependencyStatus = "satisfied"
	SessionDependencyStatusFailed    SessionDependencyStatus = "failed"
)

type TaskEventType string

const (
	TaskEventTypeSessionSpawned     TaskEventType = "session_spawned"
	TaskEventTypeSessionCompleted   TaskEventType = "session_completed"
	TaskEventTypeDependencyResolved TaskEventType = "dependency_resolved"
	TaskEventTypeContextUpdated     TaskEventType = "context_updated"
	TaskEventTypeTaskPaused         TaskEventType = "task_paused"
	TaskEventTypeTaskResumed        TaskEventType = "task_resumed"
)

// Request/Response types for API

type TaskCreateRequest struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	TaskType    TaskType               `json:"task_type"`
	Priority    int                    `json:"priority,omitempty"`
	AppID       string                 `json:"app_id,omitempty"`
	Config      map[string]interface{} `json:"config,omitempty"`
	Constraints map[string]interface{} `json:"constraints,omitempty"`
	DeadlineAt  *time.Time             `json:"deadline_at,omitempty"`
	Labels      map[string]interface{} `json:"labels,omitempty"`
}

type WorkSessionCreateRequest struct {
	TaskID              string                 `json:"task_id"`
	Name                string                 `json:"name,omitempty"`
	Description         string                 `json:"description,omitempty"`
	WorkType            WorkSessionType        `json:"work_type"`
	ParentWorkSessionID string                 `json:"parent_work_session_id,omitempty"`
	AgentConfig         map[string]interface{} `json:"agent_config,omitempty"`
	EnvironmentConfig   map[string]interface{} `json:"environment_config,omitempty"`
}

type ZedThreadCreateRequest struct {
	WorkSessionID   string                 `json:"work_session_id"`
	ProjectPath     string                 `json:"project_path,omitempty"`
	WorkspaceConfig map[string]interface{} `json:"workspace_config,omitempty"`
}

type TaskContextCreateRequest struct {
	TaskID            string                 `json:"task_id"`
	ContextType       TaskContextType        `json:"context_type"`
	ContextKey        string                 `json:"context_key"`
	ContextData       map[string]interface{} `json:"context_data"`
	ExpiresAt         *time.Time             `json:"expires_at,omitempty"`
	AccessPermissions map[string]interface{} `json:"access_permissions,omitempty"`
}

type SessionDependencyCreateRequest struct {
	TaskID              string                 `json:"task_id"`
	DependentSessionID  string                 `json:"dependent_session_id"`
	DependencySessionID string                 `json:"dependency_session_id"`
	DependencyType      SessionDependencyType  `json:"dependency_type"`
	ConditionConfig     map[string]interface{} `json:"condition_config,omitempty"`
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
	TaskID           string    `json:"task_id"`
	TaskName         string    `json:"task_name"`
	TaskStatus       string    `json:"task_status"`
	TaskType         string    `json:"task_type"`
	WorkSessionCount int       `json:"work_session_count"`
	ZedThreadCount   int       `json:"zed_thread_count"`
	ContextItems     int       `json:"context_items"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type ActiveWorkSessionResponse struct {
	WorkSessionID     string     `json:"work_session_id"`
	TaskID            string     `json:"task_id"`
	WorkSessionName   string     `json:"work_session_name"`
	WorkSessionStatus string     `json:"work_session_status"`
	HelixSessionID    string     `json:"helix_session_id"`
	HelixSessionName  string     `json:"helix_session_name"`
	ZedSessionID      string     `json:"zed_session_id,omitempty"`
	ZedThreadID       string     `json:"zed_thread_id,omitempty"`
	ZedStatus         string     `json:"zed_status,omitempty"`
	CreatedAt         time.Time  `json:"created_at"`
	StartedAt         *time.Time `json:"started_at,omitempty"`
	LastActivityAt    *time.Time `json:"last_activity_at,omitempty"`
}
