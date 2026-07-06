package types

import (
	"time"

	"gorm.io/gorm"
)

// PromptHistoryEntry represents a user's prompt in the history
// Stored per-user, per-spec-task (within a project) for cross-device sync
type PromptHistoryEntry struct {
	// Composite primary key: ID is globally unique, but we also index by user+spec_task
	ID             string `json:"id" gorm:"primaryKey;size:255"`
	UserID         string `json:"user_id" gorm:"not null;size:255;index:idx_prompt_history_user_task"`
	OrganizationID string `json:"organization_id" gorm:"size:255;index"` // Organization scope for search
	ProjectID      string `json:"project_id" gorm:"size:255;index"` // For reference, but primary grouping is by spec_task
	// SpecTaskID is nullable: frontend queue-mode messages always carry it, but
	// automated/system and general session sends (e.g. org bots via
	// POST /sessions/{id}/messages) enqueue by SessionID with no spec task.
	SpecTaskID string `json:"spec_task_id" gorm:"size:255;index:idx_prompt_history_user_task"`
	SessionID  string `json:"session_id" gorm:"size:255;index"` // Which session this was sent to (the delivery unit)

	// Content
	Content string `json:"content" gorm:"type:text;not null"`

	// NotifyUserID, when set, is the user who should be streamed the agent's
	// response (e.g. a design-review commenter). At dispatch the queue registers
	// requestToCommenterMapping/sessionToCommenterMapping from this field — the
	// same routing the old direct send set up synchronously.
	NotifyUserID string `json:"notify_user_id,omitempty" gorm:"size:255"`

	// Status tracks whether this was successfully sent
	// Values: "pending", "sent", "failed"
	Status string `json:"status" gorm:"size:50;not null;default:sent"`

	// Retry tracking for failed prompts
	RetryCount   int        `json:"retry_count" gorm:"not null;default:0"`     // Number of retry attempts
	NextRetryAt  *time.Time `json:"next_retry_at,omitempty" gorm:"index"`      // When to retry (for exponential backoff)
	ErrorMessage string     `json:"error_message,omitempty" gorm:"type:text"`  // Last failure reason (server-side error string), shown in UI under "Failed - retrying"

	// Interrupt indicates this message should interrupt the current conversation
	// When false, message waits until current conversation completes
	// Default is false: queue mode is the default, interrupt is explicit
	Interrupt bool `json:"interrupt" gorm:"not null;default:false"`

	// QueuePosition tracks ordering for drag-and-drop reordering
	// Lower values = earlier in queue. Null for sent messages.
	QueuePosition *int `json:"queue_position,omitempty" gorm:"index"`

	// Library features for prompt reuse
	Pinned     bool       `json:"pinned" gorm:"not null;default:false;index"`      // User pinned this prompt
	UsageCount int        `json:"usage_count" gorm:"not null;default:0"`           // How many times reused
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`                          // Last time reused
	Tags       string     `json:"tags,omitempty" gorm:"type:text"`                 // JSON array of user-defined tags
	IsTemplate bool       `json:"is_template" gorm:"not null;default:false;index"` // Saved as a reusable template

	// Timestamps
	CreatedAt time.Time  `json:"created_at" gorm:"not null;index"`
	UpdatedAt time.Time  `json:"updated_at" gorm:"not null"`
	DeletedAt *time.Time `json:"deleted_at,omitempty" gorm:"index"` // Soft-delete: non-nil means user removed from queue
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
	ID            string `json:"id"`
	SessionID     string `json:"session_id,omitempty"`
	Content       string `json:"content"`
	Status        string `json:"status"`
	Timestamp     int64  `json:"timestamp"`                // Unix timestamp in milliseconds
	Interrupt     *bool  `json:"interrupt,omitempty"`      // If true, interrupts current conversation
	QueuePosition *int   `json:"queue_position,omitempty"` // Position in queue for drag-and-drop ordering
	Pinned        *bool  `json:"pinned,omitempty"`         // If true, pinned by user
	Tags          string `json:"tags,omitempty"`           // JSON array of tags
	IsTemplate    *bool  `json:"is_template,omitempty"`    // If true, saved as a reusable template
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
	Synced   int                  `json:"synced"`   // Number of entries synced
	Existing int                  `json:"existing"` // Number that already existed
	Entries  []PromptHistoryEntry `json:"entries"`  // All entries for this user+project (for client merge)
}

// UnifiedSearchRequest is the request for searching across all Helix entities
type UnifiedSearchRequest struct {
	Query   string   `json:"query"`              // Search query string
	Types   []string `json:"types,omitempty"`    // Filter by types: "projects", "tasks", "sessions", "prompts"
	Limit   int      `json:"limit,omitempty"`    // Max results per type (default 10)
	OrgID   string   `json:"org_id,omitempty"`   // Optional org scope
	OwnerID string   `json:"owner_id,omitempty"` // Optional owner filter
}

// UnifiedSearchResult represents a single search result
type UnifiedSearchResult struct {
	Type        string            `json:"type"`                  // "project", "task", "session", "prompt"
	ID          string            `json:"id"`                    // Entity ID
	Title       string            `json:"title"`                 // Display title
	Description string            `json:"description,omitempty"` // Brief description/content preview
	URL         string            `json:"url"`                   // Frontend URL to navigate to
	Icon        string            `json:"icon,omitempty"`        // Icon hint for UI
	Metadata    map[string]string `json:"metadata,omitempty"`    // Additional context (status, owner, etc)
	Score       float64           `json:"score,omitempty"`       // Relevance score
	CreatedAt   string            `json:"created_at,omitempty"`  // ISO timestamp
	UpdatedAt   string            `json:"updated_at,omitempty"`  // ISO timestamp
}

// UnifiedSearchResponse is the response for unified search
type UnifiedSearchResponse struct {
	Results []UnifiedSearchResult `json:"results"`
	Total   int                   `json:"total"` // Total results across all types
	Query   string                `json:"query"` // Echo back query
}
