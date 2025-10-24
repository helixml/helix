package types

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Extensions to existing SpecTask for multi-session support
type SpecTaskMultiSessionExtensions struct {
	// Multi-session support fields - these should be added to existing SpecTask struct
	ZedInstanceID   string         `json:"zed_instance_id,omitempty" gorm:"size:255;index"`
	ProjectPath     string         `json:"project_path,omitempty" gorm:"size:500"`
	WorkspaceConfig datatypes.JSON `json:"workspace_config,omitempty" gorm:"type:jsonb"`

	// Relationships (loaded via joins, not stored)
	WorkSessions []SpecTaskWorkSession `json:"work_sessions,omitempty" gorm:"foreignKey:SpecTaskID"`
	ZedThreads   []SpecTaskZedThread   `json:"zed_threads,omitempty" gorm:"foreignKey:SpecTaskID"`
}

// SpecTaskWorkSession represents an individual work unit within a SpecTask
// Maps 1:1 to a Helix Session during implementation phase
type SpecTaskWorkSession struct {
	ID             string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID     string `json:"spec_task_id" gorm:"not null;size:255;index"`
	HelixSessionID string `json:"helix_session_id" gorm:"not null;size:255;uniqueIndex"` // 1:1 mapping

	// Work session details
	Name        string                    `json:"name,omitempty" gorm:"size:255"`
	Description string                    `json:"description,omitempty" gorm:"type:text"`
	Phase       SpecTaskPhase             `json:"phase" gorm:"not null;size:50;index"`
	Status      SpecTaskWorkSessionStatus `json:"status" gorm:"not null;size:50;default:pending;index"`

	// Implementation context (parsed from ImplementationPlan)
	ImplementationTaskTitle       string `json:"implementation_task_title,omitempty" gorm:"size:255"`
	ImplementationTaskDescription string `json:"implementation_task_description,omitempty" gorm:"type:text"`
	ImplementationTaskIndex       int    `json:"implementation_task_index,omitempty"`

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

	// Relationships (loaded via joins, not stored in database)
	// NOTE: We store IDs, not nested objects, to avoid circular references in Swagger/JSON
	// Use GORM preloading to load these relationships when needed:
	//   db.Preload("HelixSession").Find(&workSession)
	// swaggerignore prevents circular reference in swagger generation
	HelixSession      *Session              `json:"helix_session,omitempty" gorm:"foreignKey:HelixSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE" swaggerignore:"true"`
	ParentWorkSession *SpecTaskWorkSession  `json:"parent_work_session,omitempty" gorm:"foreignKey:ParentWorkSessionID" swaggerignore:"true"`
	SpawnedBySession  *SpecTaskWorkSession  `json:"spawned_by_session,omitempty" gorm:"foreignKey:SpawnedBySessionID" swaggerignore:"true"`
	ZedThread         *SpecTaskZedThread    `json:"zed_thread,omitempty" gorm:"foreignKey:WorkSessionID" swaggerignore:"true"`
	ChildWorkSessions []SpecTaskWorkSession `json:"child_work_sessions,omitempty" gorm:"foreignKey:ParentWorkSessionID" swaggerignore:"true"`
}

// SpecTaskZedThread maps individual work sessions to threads within a Zed instance
type SpecTaskZedThread struct {
	ID            string `json:"id" gorm:"primaryKey;size:255"`
	WorkSessionID string `json:"work_session_id" gorm:"not null;size:255;uniqueIndex"` // 1:1 mapping
	SpecTaskID    string `json:"spec_task_id" gorm:"not null;size:255;index"`
	ZedThreadID   string `json:"zed_thread_id" gorm:"not null;size:255;index"`

	// Thread-specific configuration
	ThreadConfig   datatypes.JSON    `json:"thread_config,omitempty" gorm:"type:jsonb"`
	Status         SpecTaskZedStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
	LastActivityAt *time.Time        `json:"last_activity_at,omitempty"`

	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Relationships (loaded via joins, not stored in database)
	// NOTE: We store IDs, not nested objects, to avoid circular references
	// Use GORM preloading: db.Preload("WorkSession").Find(&zedThread)
	WorkSession *SpecTaskWorkSession `json:"work_session,omitempty" gorm:"foreignKey:WorkSessionID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
}

// SpecTaskImplementationTask represents parsed tasks from ImplementationPlan
type SpecTaskImplementationTask struct {
	ID         string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`

	// Task details
	Title              string         `json:"title" gorm:"not null;size:255"`
	Description        string         `json:"description" gorm:"type:text"`
	AcceptanceCriteria string         `json:"acceptance_criteria" gorm:"type:text"`
	EstimatedEffort    string         `json:"estimated_effort" gorm:"size:50"` // 'small', 'medium', 'large'
	Priority           int            `json:"priority" gorm:"default:0"`
	Index              int            `json:"index" gorm:"not null"`          // Order within the plan
	Dependencies       datatypes.JSON `json:"dependencies" gorm:"type:jsonb"` // Array of other task indices

	// Implementation tracking
	Status                SpecTaskImplementationStatus `json:"status" gorm:"not null;size:50;default:pending;index"`
	AssignedWorkSessionID string                       `json:"assigned_work_session_id,omitempty" gorm:"size:255;index"`

	CreatedAt   time.Time  `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relationships
	SpecTask            *SpecTask            `json:"spec_task,omitempty" gorm:"foreignKey:SpecTaskID;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	AssignedWorkSession *SpecTaskWorkSession `json:"assigned_work_session,omitempty" gorm:"foreignKey:AssignedWorkSessionID"`
}

// Enums

type SpecTaskPhase string

const (
	SpecTaskPhasePlanning       SpecTaskPhase = "planning"
	SpecTaskPhaseImplementation SpecTaskPhase = "implementation"
	SpecTaskPhaseValidation     SpecTaskPhase = "validation"
)

type SpecTaskWorkSessionStatus string

const (
	SpecTaskWorkSessionStatusPending   SpecTaskWorkSessionStatus = "pending"
	SpecTaskWorkSessionStatusActive    SpecTaskWorkSessionStatus = "active"
	SpecTaskWorkSessionStatusCompleted SpecTaskWorkSessionStatus = "completed"
	SpecTaskWorkSessionStatusFailed    SpecTaskWorkSessionStatus = "failed"
	SpecTaskWorkSessionStatusCancelled SpecTaskWorkSessionStatus = "cancelled"
	SpecTaskWorkSessionStatusBlocked   SpecTaskWorkSessionStatus = "blocked"
)

type SpecTaskZedStatus string

const (
	SpecTaskZedStatusPending      SpecTaskZedStatus = "pending"
	SpecTaskZedStatusActive       SpecTaskZedStatus = "active"
	SpecTaskZedStatusDisconnected SpecTaskZedStatus = "disconnected"
	SpecTaskZedStatusCompleted    SpecTaskZedStatus = "completed"
	SpecTaskZedStatusFailed       SpecTaskZedStatus = "failed"
)

type SpecTaskImplementationStatus string

const (
	SpecTaskImplementationStatusPending    SpecTaskImplementationStatus = "pending"
	SpecTaskImplementationStatusAssigned   SpecTaskImplementationStatus = "assigned"
	SpecTaskImplementationStatusInProgress SpecTaskImplementationStatus = "in_progress"
	SpecTaskImplementationStatusCompleted  SpecTaskImplementationStatus = "completed"
	SpecTaskImplementationStatusBlocked    SpecTaskImplementationStatus = "blocked"
)

// Request types for API

type SpecTaskWorkSessionCreateRequest struct {
	SpecTaskID              string                 `json:"spec_task_id" validate:"required"`
	Name                    string                 `json:"name,omitempty" validate:"omitempty,max=255"`
	Description             string                 `json:"description,omitempty"`
	Phase                   SpecTaskPhase          `json:"phase" validate:"required,oneof=planning implementation validation"`
	ImplementationTaskIndex int                    `json:"implementation_task_index,omitempty"`
	ParentWorkSessionID     string                 `json:"parent_work_session_id,omitempty"`
	AgentConfig             map[string]interface{} `json:"agent_config,omitempty"`
	EnvironmentConfig       map[string]interface{} `json:"environment_config,omitempty"`
}

type SpecTaskWorkSessionUpdateRequest struct {
	Name        string                    `json:"name,omitempty" validate:"omitempty,max=255"`
	Description string                    `json:"description,omitempty"`
	Status      SpecTaskWorkSessionStatus `json:"status,omitempty" validate:"omitempty,oneof=pending active completed failed cancelled blocked"`
	Config      map[string]interface{}    `json:"config,omitempty"`
}

type SpecTaskZedThreadCreateRequest struct {
	WorkSessionID string                 `json:"work_session_id" validate:"required"`
	ThreadConfig  map[string]interface{} `json:"thread_config,omitempty"`
}

type SpecTaskZedThreadUpdateRequest struct {
	Status       SpecTaskZedStatus      `json:"status,omitempty" validate:"omitempty,oneof=pending active disconnected completed failed"`
	ThreadConfig map[string]interface{} `json:"thread_config,omitempty"`
}

type SpecTaskImplementationSessionsCreateRequest struct {
	SpecTaskID         string                 `json:"spec_task_id" validate:"required"`
	ProjectPath        string                 `json:"project_path,omitempty"`
	WorkspaceConfig    map[string]interface{} `json:"workspace_config,omitempty"`
	AutoCreateSessions bool                   `json:"auto_create_sessions" default:"true"`
}

type SpecTaskWorkSessionSpawnRequest struct {
	ParentWorkSessionID string                 `json:"parent_work_session_id" validate:"required"`
	Name                string                 `json:"name" validate:"required,max=255"`
	Description         string                 `json:"description,omitempty"`
	AgentConfig         map[string]interface{} `json:"agent_config,omitempty"`
	EnvironmentConfig   map[string]interface{} `json:"environment_config,omitempty"`
}

// Response types

type SpecTaskWorkSessionListResponse struct {
	WorkSessions []SpecTaskWorkSession `json:"work_sessions"`
	Total        int                   `json:"total"`
}

type SpecTaskZedThreadListResponse struct {
	ZedThreads []SpecTaskZedThread `json:"zed_threads"`
	Total      int                 `json:"total"`
}

type SpecTaskImplementationTaskListResponse struct {
	ImplementationTasks []SpecTaskImplementationTask `json:"implementation_tasks"`
	Total               int                          `json:"total"`
}

type SpecTaskWorkSessionDetailResponse struct {
	WorkSession        SpecTaskWorkSession         `json:"work_session"`
	SpecTask           SpecTask                    `json:"spec_task"`
	HelixSession       *Session                    `json:"helix_session,omitempty"`
	ZedThread          *SpecTaskZedThread          `json:"zed_thread,omitempty"`
	ImplementationTask *SpecTaskImplementationTask `json:"implementation_task,omitempty"`
	ChildWorkSessions  []SpecTaskWorkSession       `json:"child_work_sessions,omitempty"`
}

type SpecTaskMultiSessionOverviewResponse struct {
	SpecTask            SpecTask                     `json:"spec_task"`
	WorkSessionCount    int                          `json:"work_session_count"`
	ActiveSessions      int                          `json:"active_sessions"`
	CompletedSessions   int                          `json:"completed_sessions"`
	ZedThreadCount      int                          `json:"zed_thread_count"`
	ZedInstanceID       string                       `json:"zed_instance_id,omitempty"`
	LastActivity        *time.Time                   `json:"last_activity,omitempty"`
	WorkSessions        []SpecTaskWorkSession        `json:"work_sessions,omitempty"`
	ImplementationTasks []SpecTaskImplementationTask `json:"implementation_tasks,omitempty"`
}

type SpecTaskProgressResponse struct {
	SpecTask               SpecTask                   `json:"spec_task"`
	OverallProgress        float64                    `json:"overall_progress"` // 0.0 to 1.0
	PhaseProgress          map[SpecTaskPhase]float64  `json:"phase_progress"`
	ImplementationProgress map[int]float64            `json:"implementation_progress"` // Task index -> progress
	ActiveWorkSessions     []SpecTaskWorkSession      `json:"active_work_sessions"`
	RecentActivity         []SpecTaskActivityLogEntry `json:"recent_activity"`
}

type SpecTaskActivityLogEntry struct {
	ID            string                 `json:"id"`
	SpecTaskID    string                 `json:"spec_task_id"`
	WorkSessionID string                 `json:"work_session_id,omitempty"`
	ActivityType  SpecTaskActivityType   `json:"activity_type"`
	Message       string                 `json:"message"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
	Timestamp     time.Time              `json:"timestamp"`
}

type SpecTaskActivityType string

const (
	SpecTaskActivitySessionCreated   SpecTaskActivityType = "session_created"
	SpecTaskActivitySessionCompleted SpecTaskActivityType = "session_completed"
	SpecTaskActivitySessionSpawned   SpecTaskActivityType = "session_spawned"
	SpecTaskActivityTaskCompleted    SpecTaskActivityType = "task_completed"
	SpecTaskActivityZedConnected     SpecTaskActivityType = "zed_connected"
	SpecTaskActivityZedDisconnected  SpecTaskActivityType = "zed_disconnected"
	SpecTaskActivityPhaseTransition  SpecTaskActivityType = "phase_transition"
)

// ZedInstanceEvent represents events from Zed instances
type ZedInstanceEvent struct {
	InstanceID string                 `json:"instance_id"`
	SpecTaskID string                 `json:"spec_task_id,omitempty"`
	ThreadID   string                 `json:"thread_id,omitempty"`
	EventType  string                 `json:"event_type"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  time.Time              `json:"timestamp"`
}

// ZedInstanceStatus represents the status of a Zed instance for a SpecTask
type ZedInstanceStatus struct {
	SpecTaskID    string     `json:"spec_task_id"`
	ZedInstanceID string     `json:"zed_instance_id,omitempty"`
	Status        string     `json:"status"`
	ThreadCount   int        `json:"thread_count"`
	ActiveThreads int        `json:"active_threads"`
	LastActivity  *time.Time `json:"last_activity,omitempty"`
	ProjectPath   string     `json:"project_path,omitempty"`
}

// GORM Hooks for validation and ID generation

func (s *SpecTaskWorkSession) BeforeCreate(tx *gorm.DB) error {
	if s.ID == "" {
		s.ID = GenerateSpecTaskWorkSessionID()
	}
	return nil
}

func (s *SpecTaskWorkSession) BeforeUpdate(tx *gorm.DB) error {
	s.UpdatedAt = time.Now()
	return nil
}

func (z *SpecTaskZedThread) BeforeCreate(tx *gorm.DB) error {
	if z.ID == "" {
		z.ID = GenerateSpecTaskZedThreadID()
	}
	return nil
}

func (z *SpecTaskZedThread) BeforeUpdate(tx *gorm.DB) error {
	z.UpdatedAt = time.Now()
	return nil
}

func (i *SpecTaskImplementationTask) BeforeCreate(tx *gorm.DB) error {
	if i.ID == "" {
		i.ID = GenerateSpecTaskImplementationTaskID()
	}
	return nil
}

// Table names
func (SpecTaskWorkSession) TableName() string {
	return "spec_task_work_sessions"
}

func (SpecTaskZedThread) TableName() string {
	return "spec_task_zed_threads"
}

func (SpecTaskImplementationTask) TableName() string {
	return "spec_task_implementation_tasks"
}

// Helper functions for ID generation
func GenerateSpecTaskWorkSessionID() string {
	return "stws_" + generateUUID()
}

func GenerateSpecTaskZedThreadID() string {
	return "stzt_" + generateUUID()
}

func GenerateSpecTaskImplementationTaskID() string {
	return "stit_" + generateUUID()
}

// generateUUID generates a simple UUID for internal use
func generateUUID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Validation methods
func (s *SpecTaskWorkSession) IsImplementationSession() bool {
	return s.Phase == SpecTaskPhaseImplementation
}

func (s *SpecTaskWorkSession) IsPlanningSession() bool {
	return s.Phase == SpecTaskPhasePlanning
}

func (s *SpecTaskWorkSession) IsZedBackedSession() bool {
	return s.ZedThread != nil
}

func (s *SpecTaskWorkSession) CanSpawnSessions() bool {
	return s.Status == SpecTaskWorkSessionStatusActive && s.IsImplementationSession()
}

func (s *SpecTaskWorkSession) IsActive() bool {
	return s.Status == SpecTaskWorkSessionStatusActive
}

func (s *SpecTaskWorkSession) IsCompleted() bool {
	return s.Status == SpecTaskWorkSessionStatusCompleted
}

func (s *SpecTaskWorkSession) IsPending() bool {
	return s.Status == SpecTaskWorkSessionStatusPending
}

func (s *SpecTaskWorkSession) HasParent() bool {
	return s.ParentWorkSessionID != ""
}

func (s *SpecTaskWorkSession) WasSpawned() bool {
	return s.SpawnedBySessionID != ""
}

func (z *SpecTaskZedThread) IsActive() bool {
	return z.Status == SpecTaskZedStatusActive
}

func (z *SpecTaskZedThread) IsCompleted() bool {
	return z.Status == SpecTaskZedStatusCompleted
}

func (z *SpecTaskZedThread) IsDisconnected() bool {
	return z.Status == SpecTaskZedStatusDisconnected
}

func (z *SpecTaskZedThread) HasRecentActivity(threshold time.Duration) bool {
	if z.LastActivityAt == nil {
		return false
	}
	return time.Since(*z.LastActivityAt) <= threshold
}

func (i *SpecTaskImplementationTask) IsCompleted() bool {
	return i.Status == SpecTaskImplementationStatusCompleted
}

func (i *SpecTaskImplementationTask) CanBeAssigned() bool {
	return i.Status == SpecTaskImplementationStatusPending
}

func (i *SpecTaskImplementationTask) IsAssigned() bool {
	return i.Status == SpecTaskImplementationStatusAssigned && i.AssignedWorkSessionID != ""
}

func (i *SpecTaskImplementationTask) IsInProgress() bool {
	return i.Status == SpecTaskImplementationStatusInProgress
}

func (i *SpecTaskImplementationTask) IsBlocked() bool {
	return i.Status == SpecTaskImplementationStatusBlocked
}

func (i *SpecTaskImplementationTask) HasDependencies() bool {
	var deps []int
	if len(i.Dependencies) > 0 {
		if err := json.Unmarshal(i.Dependencies, &deps); err == nil {
			return len(deps) > 0
		}
	}
	return false
}
