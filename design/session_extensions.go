package types

import (
	"time"

	"gorm.io/gorm"
)

// Extensions to the existing Session struct for task integration
// These fields should be added to the existing Session struct in types.go

type SessionExtensions struct {
	// Task integration fields - add these to existing Session struct
	TaskID        string `json:"task_id,omitempty" gorm:"size:255;index"`
	WorkSessionID string `json:"work_session_id,omitempty" gorm:"size:255;index"`

	// Optional: Session role for clarity
	SessionRole SessionRole `json:"session_role" gorm:"size:50;default:standalone;index"`
}

type SessionRole string

const (
	SessionRoleStandalone      SessionRole = "standalone"       // Regular session not part of task
	SessionRoleTaskCoordinator SessionRole = "task_coordinator" // Main coordination session for task
	SessionRoleWorkSession     SessionRole = "work_session"     // Individual work session within task
)

// GORM hooks to add to Session struct for task integration

func (s *Session) BeforeCreate(tx *gorm.DB) error {
	// Set session role based on task/work session assignment
	if s.WorkSessionID != "" {
		s.SessionRole = SessionRoleWorkSession
	} else if s.TaskID != "" {
		s.SessionRole = SessionRoleTaskCoordinator
	} else {
		s.SessionRole = SessionRoleStandalone
	}

	return nil
}

func (s *Session) BeforeUpdate(tx *gorm.DB) error {
	// Update session role if task/work session assignment changes
	if s.WorkSessionID != "" {
		s.SessionRole = SessionRoleWorkSession
	} else if s.TaskID != "" {
		s.SessionRole = SessionRoleTaskCoordinator
	} else {
		s.SessionRole = SessionRoleStandalone
	}

	return nil
}

// Helper methods to add to Session struct

func (s *Session) IsTaskSession() bool {
	return s.TaskID != "" || s.WorkSessionID != ""
}

func (s *Session) IsWorkSession() bool {
	return s.WorkSessionID != ""
}

func (s *Session) IsTaskCoordinator() bool {
	return s.TaskID != "" && s.WorkSessionID == ""
}

func (s *Session) GetTaskContext(tx *gorm.DB) (*Task, error) {
	if s.TaskID == "" {
		return nil, nil
	}

	var task Task
	err := tx.First(&task, "id = ?", s.TaskID).Error
	if err != nil {
		return nil, err
	}

	return &task, nil
}

func (s *Session) GetWorkSessionContext(tx *gorm.DB) (*WorkSession, error) {
	if s.WorkSessionID == "" {
		return nil, nil
	}

	var workSession WorkSession
	err := tx.Preload("Task").First(&workSession, "id = ?", s.WorkSessionID).Error
	if err != nil {
		return nil, err
	}

	return &workSession, nil
}

// Extended session metadata for task context
type SessionTaskMetadata struct {
	// Task-specific context
	TaskName        string `json:"task_name,omitempty"`
	TaskType        string `json:"task_type,omitempty"`
	WorkSessionName string `json:"work_session_name,omitempty"`
	AgentType       string `json:"agent_type,omitempty"`

	// Hierarchy context
	ParentWorkSessionID string `json:"parent_work_session_id,omitempty"`
	SpawnedBySessionID  string `json:"spawned_by_session_id,omitempty"`

	// Zed integration
	ZedSessionID string `json:"zed_session_id,omitempty"`
	ZedThreadID  string `json:"zed_thread_id,omitempty"`
	ProjectPath  string `json:"project_path,omitempty"`
}

// Method to populate task metadata in session
func (s *Session) PopulateTaskMetadata(tx *gorm.DB) (*SessionTaskMetadata, error) {
	metadata := &SessionTaskMetadata{}

	if s.WorkSessionID != "" {
		var workSession WorkSession
		err := tx.Preload("Task").Preload("ZedThreadMapping").First(&workSession, "id = ?", s.WorkSessionID).Error
		if err != nil {
			return nil, err
		}

		metadata.WorkSessionName = workSession.Name
		metadata.AgentType = string(workSession.AgentType)
		metadata.ParentWorkSessionID = workSession.ParentWorkSessionID
		metadata.SpawnedBySessionID = workSession.SpawnedBySessionID

		if workSession.Task != nil {
			metadata.TaskName = workSession.Task.Name
			metadata.TaskType = string(workSession.Task.TaskType)
		}

		if workSession.ZedThreadMapping != nil {
			metadata.ZedSessionID = workSession.ZedThreadMapping.ZedSessionID
			metadata.ZedThreadID = workSession.ZedThreadMapping.ZedThreadID
			metadata.ProjectPath = workSession.ZedThreadMapping.ProjectPath
		}
	} else if s.TaskID != "" {
		var task Task
		err := tx.First(&task, "id = ?", s.TaskID).Error
		if err != nil {
			return nil, err
		}

		metadata.TaskName = task.Name
		metadata.TaskType = string(task.TaskType)
	}

	return metadata, nil
}

// Session factory methods for task integration

func CreateTaskCoordinatorSession(task *Task, userID string) *Session {
	session := &Session{
		// Copy relevant fields from task
		Owner:          userID,
		OrganizationID: task.OrganizationID,
		ParentApp:      task.AppID,
		TaskID:         task.ID,
		SessionRole:    SessionRoleTaskCoordinator,

		// Session-specific defaults
		Name: "Task Coordinator: " + task.Name,
		Type: SessionTypeText,
		Mode: SessionModeInference,

		// Timestamps
		Created: time.Now(),
		Updated: time.Now(),
	}

	return session
}

func CreateWorkSession(workSession *WorkSession, userID string) *Session {
	session := &Session{
		// Copy relevant fields from work session
		Owner:         userID,
		TaskID:        workSession.TaskID,
		WorkSessionID: workSession.ID,
		SessionRole:   SessionRoleWorkSession,

		// Session-specific defaults
		Name: workSession.Name,
		Type: SessionTypeText,
		Mode: SessionModeInference,

		// Timestamps
		Created: time.Now(),
		Updated: time.Now(),
	}

	// Set agent-specific configuration
	if workSession.AgentType == AgentTypeZedAgent {
		// Configure for Zed integration
		session.Metadata.AgentType = "zed_external"
	}

	return session
}

// Query helpers for task-related sessions

func GetTaskSessions(tx *gorm.DB, taskID string) ([]Session, error) {
	var sessions []Session
	err := tx.Where("task_id = ?", taskID).Find(&sessions).Error
	return sessions, err
}

func GetWorkSessionsForTask(tx *gorm.DB, taskID string) ([]Session, error) {
	var sessions []Session
	err := tx.Where("task_id = ? AND work_session_id IS NOT NULL", taskID).Find(&sessions).Error
	return sessions, err
}

func GetActiveTaskSessions(tx *gorm.DB, userID string) ([]Session, error) {
	var sessions []Session
	err := tx.Joins("JOIN tasks ON sessions.task_id = tasks.id").
		Where("sessions.owner = ? AND tasks.status = ?", userID, TaskStatusActive).
		Find(&sessions).Error
	return sessions, err
}
