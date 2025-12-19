package types

import (
	"time"

	"gorm.io/gorm"
)

// PromptHistoryEntry represents a user's prompt in the history
// Stored per-user, per-spec-task (within a project) for cross-device sync
type PromptHistoryEntry struct {
	// Composite primary key: ID is globally unique, but we also index by user+spec_task
	ID         string    `json:"id" gorm:"primaryKey;size:255"`
	UserID     string    `json:"user_id" gorm:"not null;size:255;index:idx_prompt_history_user_task"`
	ProjectID  string    `json:"project_id" gorm:"not null;size:255;index"` // For reference, but primary grouping is by spec_task
	SpecTaskID string    `json:"spec_task_id" gorm:"not null;size:255;index:idx_prompt_history_user_task"`
	SessionID  string    `json:"session_id" gorm:"size:255;index"` // Optional - which session this was sent to

	// Content
	Content string `json:"content" gorm:"type:text;not null"`

	// Status tracks whether this was successfully sent
	// Values: "pending", "sent", "failed"
	Status string `json:"status" gorm:"size:50;not null;default:sent"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null;index"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null"`
}

// BeforeCreate sets up the entry before creation
func (p *PromptHistoryEntry) BeforeCreate(tx *gorm.DB) error {
	if p.CreatedAt.IsZero() {
		p.CreatedAt = time.Now()
	}
	p.UpdatedAt = time.Now()
	return nil
}

// BeforeUpdate updates the timestamp
func (p *PromptHistoryEntry) BeforeUpdate(tx *gorm.DB) error {
	p.UpdatedAt = time.Now()
	return nil
}

// PromptHistorySyncRequest is the request body for syncing prompt history from frontend
type PromptHistorySyncRequest struct {
	ProjectID  string                   `json:"project_id"`
	SpecTaskID string                   `json:"spec_task_id"`
	Entries    []PromptHistoryEntrySync `json:"entries"`
}

// PromptHistoryEntrySync is a single entry in the sync request
type PromptHistoryEntrySync struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id,omitempty"`
	Content   string `json:"content"`
	Status    string `json:"status"`
	Timestamp int64  `json:"timestamp"` // Unix timestamp in milliseconds
}

// PromptHistoryListRequest is the query parameters for listing history
type PromptHistoryListRequest struct {
	ProjectID  string `json:"project_id"`
	SpecTaskID string `json:"spec_task_id"`         // Required - history is per-spec-task
	SessionID  string `json:"session_id,omitempty"` // Optional filter
	Limit      int    `json:"limit,omitempty"`      // Max entries to return
	Since      int64  `json:"since,omitempty"`      // Only entries after this timestamp (Unix ms)
}

// PromptHistoryListResponse is the response for listing history
type PromptHistoryListResponse struct {
	Entries []PromptHistoryEntry `json:"entries"`
	Total   int64                `json:"total"`
}

// PromptHistorySyncResponse is the response after syncing
type PromptHistorySyncResponse struct {
	Synced   int                    `json:"synced"`   // Number of entries synced
	Existing int                    `json:"existing"` // Number that already existed
	Entries  []PromptHistoryEntry   `json:"entries"`  // All entries for this user+project (for client merge)
}
