package types

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
)

// AttentionEvent represents an event that requires human attention.
// Events are created by the backend when something happens that a user should
// look at (specs pushed, agent interaction completed, failure, etc.).
type AttentionEvent struct {
	ID             string             `json:"id" gorm:"primaryKey;size:255"`
	UserID         string             `json:"user_id" gorm:"not null;size:255;index"`
	OrganizationID string             `json:"organization_id" gorm:"not null;size:255;index"`
	ProjectID      string             `json:"project_id" gorm:"not null;size:255;index"`
	SpecTaskID     string             `json:"spec_task_id" gorm:"not null;size:255;index"`
	EventType      AttentionEventType `json:"event_type" gorm:"not null;size:50;index"`
	Title          string             `json:"title" gorm:"not null;size:500"`
	Description    string             `json:"description,omitempty" gorm:"type:text"`
	CreatedAt      time.Time          `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	AcknowledgedAt *time.Time         `json:"acknowledged_at,omitempty"`
	DismissedAt    *time.Time         `json:"dismissed_at,omitempty" gorm:"index"`
	SnoozedUntil   *time.Time         `json:"snoozed_until,omitempty"`
	IdempotencyKey string             `json:"idempotency_key,omitempty" gorm:"size:500;uniqueIndex"`
	Metadata       datatypes.JSON     `json:"metadata,omitempty" gorm:"type:jsonb"`

	// Denormalized for display without joins
	ProjectName  string `json:"project_name,omitempty" gorm:"size:255"`
	SpecTaskName string `json:"spec_task_name,omitempty" gorm:"size:500"`
}

type AttentionEventType string

const (
	AttentionEventSpecsPushed               AttentionEventType = "specs_pushed"
	AttentionEventAgentInteractionCompleted AttentionEventType = "agent_interaction_completed"
	AttentionEventSpecFailed                AttentionEventType = "spec_failed"
	AttentionEventImplementationFailed      AttentionEventType = "implementation_failed"
	AttentionEventPRReady                   AttentionEventType = "pr_ready"
)

// AttentionEventUpdateRequest is the request body for updating an attention event
// (acknowledge, dismiss, or snooze).
type AttentionEventUpdateRequest struct {
	Acknowledge  bool       `json:"acknowledge,omitempty"`
	Dismiss      bool       `json:"dismiss,omitempty"`
	SnoozedUntil *time.Time `json:"snoozed_until,omitempty"`
}

// BuildAttentionEventIdempotencyKey creates a unique key to prevent duplicate events.
func BuildAttentionEventIdempotencyKey(taskID string, eventType AttentionEventType, qualifier string) string {
	if qualifier != "" {
		return fmt.Sprintf("%s:%s:%s", taskID, eventType, qualifier)
	}
	return fmt.Sprintf("%s:%s", taskID, eventType)
}
