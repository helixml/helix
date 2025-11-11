package types

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// SpecTaskDesignReview represents a complete review of design documents
// Created when agent pushes design docs to Git
type SpecTaskDesignReview struct {
	ID         string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`

	// Review metadata
	ReviewerID string                    `json:"reviewer_id" gorm:"size:255;index"` // User who is reviewing
	Status     SpecTaskDesignReviewStatus `json:"status" gorm:"not null;size:50;default:pending;index"`

	// Git information
	GitCommitHash string    `json:"git_commit_hash" gorm:"size:255;index"` // SHA of the design doc commit
	GitBranch     string    `json:"git_branch" gorm:"size:255"`
	GitPushedAt   time.Time `json:"git_pushed_at"`

	// Design document snapshots at time of review
	RequirementsSpec   string `json:"requirements_spec" gorm:"type:text"`
	TechnicalDesign    string `json:"technical_design" gorm:"type:text"`
	ImplementationPlan string `json:"implementation_plan" gorm:"type:text"`

	// Review decision
	OverallComment string    `json:"overall_comment" gorm:"type:text"`
	ApprovedAt     *time.Time `json:"approved_at,omitempty"`
	RejectedAt     *time.Time `json:"rejected_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Relationships
	Comments []SpecTaskDesignReviewComment `json:"comments,omitempty" gorm:"foreignKey:ReviewID;constraint:OnDelete:CASCADE" swaggerignore:"true"`
	SpecTask *SpecTask                     `json:"spec_task,omitempty" gorm:"foreignKey:SpecTaskID;constraint:OnDelete:CASCADE" swaggerignore:"true"`
}

// SpecTaskDesignReviewComment represents a comment on a specific section of a design document
type SpecTaskDesignReviewComment struct {
	ID       string `json:"id" gorm:"primaryKey;size:255"`
	ReviewID string `json:"review_id" gorm:"not null;size:255;index"`

	// Comment metadata
	CommentedBy string `json:"commented_by" gorm:"not null;size:255;index"` // User who made the comment

	// Location in document
	DocumentType string `json:"document_type" gorm:"not null;size:50;index"` // "requirements", "technical_design", "implementation_plan"
	SectionPath  string `json:"section_path" gorm:"type:text"`                // e.g., "## Architecture/### Database Schema"
	LineNumber   int    `json:"line_number,omitempty"`                        // Optional line number

	// For inline comments - store the context around the comment
	QuotedText   string `json:"quoted_text,omitempty" gorm:"type:text"`  // Text being commented on
	StartOffset  int    `json:"start_offset,omitempty"`                  // Character offset in document
	EndOffset    int    `json:"end_offset,omitempty"`                    // Character offset in document

	// The actual comment
	CommentText string                         `json:"comment_text" gorm:"type:text;not null"`
	CommentType SpecTaskDesignReviewCommentType `json:"comment_type,omitempty" gorm:"size:50"` // Made optional - simplified to single type

	// Agent integration (NEW FIELDS)
	AgentResponse   string     `json:"agent_response,omitempty" gorm:"type:text"`       // Agent's response to comment
	AgentResponseAt *time.Time `json:"agent_response_at,omitempty"`                     // When agent responded
	InteractionID   string     `json:"interaction_id,omitempty" gorm:"size:255;index"` // Link to Helix interaction

	// Status tracking
	Resolved         bool       `json:"resolved" gorm:"default:false"`
	ResolvedBy       string     `json:"resolved_by,omitempty" gorm:"size:255"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	ResolutionReason string     `json:"resolution_reason,omitempty" gorm:"size:100"` // "manual", "auto_text_removed", "agent_updated"

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`

	// Relationships
	Review *SpecTaskDesignReview `json:"review,omitempty" gorm:"foreignKey:ReviewID" swaggerignore:"true"`
	Replies []SpecTaskDesignReviewCommentReply `json:"replies,omitempty" gorm:"foreignKey:CommentID;constraint:OnDelete:CASCADE" swaggerignore:"true"`
}

// SpecTaskDesignReviewCommentReply represents a threaded reply to a comment
type SpecTaskDesignReviewCommentReply struct {
	ID        string `json:"id" gorm:"primaryKey;size:255"`
	CommentID string `json:"comment_id" gorm:"not null;size:255;index"`

	// Reply metadata
	RepliedBy string `json:"replied_by" gorm:"not null;size:255;index"` // User or agent
	ReplyText string `json:"reply_text" gorm:"type:text;not null"`
	IsAgent   bool   `json:"is_agent" gorm:"default:false"` // True if agent replied

	// Timestamps
	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`

	// Relationships
	Comment *SpecTaskDesignReviewComment `json:"comment,omitempty" gorm:"foreignKey:CommentID" swaggerignore:"true"`
}

// Git push event tracking
type SpecTaskGitPushEvent struct {
	ID         string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`

	// Git details
	CommitHash   string    `json:"commit_hash" gorm:"not null;size:255;index"`
	Branch       string    `json:"branch" gorm:"not null;size:255;index"`
	AuthorName   string    `json:"author_name" gorm:"size:255"`
	AuthorEmail  string    `json:"author_email" gorm:"size:255"`
	CommitMessage string    `json:"commit_message" gorm:"type:text"`
	PushedAt     time.Time `json:"pushed_at" gorm:"not null;index"`

	// Event processing
	Processed      bool       `json:"processed" gorm:"default:false;index"`
	ProcessedAt    *time.Time `json:"processed_at,omitempty"`
	ProcessingError string    `json:"processing_error,omitempty" gorm:"type:text"`

	// Files changed (for detecting design doc updates)
	FilesChanged datatypes.JSON `json:"files_changed" gorm:"type:jsonb"` // Array of file paths

	// Metadata
	EventSource string    `json:"event_source" gorm:"size:50"` // "webhook", "polling", "manual"
	RawPayload  datatypes.JSON `json:"raw_payload,omitempty" gorm:"type:jsonb"` // Original webhook/event data
	CreatedAt   time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
}

// Enums

type SpecTaskDesignReviewStatus string

const (
	SpecTaskDesignReviewStatusPending       SpecTaskDesignReviewStatus = "pending"         // Waiting for reviewer
	SpecTaskDesignReviewStatusInReview      SpecTaskDesignReviewStatus = "in_review"       // Reviewer is actively reviewing
	SpecTaskDesignReviewStatusChangesRequested SpecTaskDesignReviewStatus = "changes_requested" // Reviewer requested changes
	SpecTaskDesignReviewStatusApproved      SpecTaskDesignReviewStatus = "approved"        // Approved, ready for implementation
	SpecTaskDesignReviewStatusSuperseded    SpecTaskDesignReviewStatus = "superseded"      // Newer review exists (agent pushed updates)
)

type SpecTaskDesignReviewCommentType string

const (
	SpecTaskDesignReviewCommentTypeGeneral     SpecTaskDesignReviewCommentType = "general"     // General comment
	SpecTaskDesignReviewCommentTypeQuestion    SpecTaskDesignReviewCommentType = "question"    // Question needing clarification
	SpecTaskDesignReviewCommentTypeSuggestion  SpecTaskDesignReviewCommentType = "suggestion"  // Suggested improvement
	SpecTaskDesignReviewCommentTypeCritical    SpecTaskDesignReviewCommentType = "critical"    // Critical issue must be fixed
	SpecTaskDesignReviewCommentTypePraise      SpecTaskDesignReviewCommentType = "praise"      // Positive feedback
)

// Request types

type SpecTaskDesignReviewCreateRequest struct {
	SpecTaskID    string `json:"spec_task_id" validate:"required"`
	GitCommitHash string `json:"git_commit_hash" validate:"required"`
	GitBranch     string `json:"git_branch" validate:"required"`
}

type SpecTaskDesignReviewCommentCreateRequest struct {
	ReviewID     string                          `json:"review_id" validate:"required"`
	DocumentType string                          `json:"document_type" validate:"required,oneof=requirements technical_design implementation_plan"`
	SectionPath  string                          `json:"section_path,omitempty"`
	LineNumber   int                             `json:"line_number,omitempty"`
	QuotedText   string                          `json:"quoted_text,omitempty"`
	StartOffset  int                             `json:"start_offset,omitempty"`
	EndOffset    int                             `json:"end_offset,omitempty"`
	CommentText  string                          `json:"comment_text" validate:"required"`
	CommentType  SpecTaskDesignReviewCommentType `json:"comment_type,omitempty"`
}

type SpecTaskDesignReviewCommentReplyCreateRequest struct {
	CommentID string `json:"comment_id" validate:"required"`
	ReplyText string `json:"reply_text" validate:"required"`
}

type SpecTaskDesignReviewSubmitRequest struct {
	ReviewID       string                     `json:"review_id" validate:"required"`
	Decision       string                     `json:"decision" validate:"required,oneof=approve request_changes"` // "approve" or "request_changes"
	OverallComment string                     `json:"overall_comment,omitempty"`
}

type SpecTaskDesignReviewUpdateRequest struct {
	Status         SpecTaskDesignReviewStatus `json:"status,omitempty"`
	OverallComment string                     `json:"overall_comment,omitempty"`
}

// Response types

type SpecTaskDesignReviewListResponse struct {
	Reviews []SpecTaskDesignReview `json:"reviews"`
	Total   int                    `json:"total"`
}

type SpecTaskDesignReviewDetailResponse struct {
	Review   SpecTaskDesignReview          `json:"review"`
	Comments []SpecTaskDesignReviewComment `json:"comments"`
	SpecTask SpecTask                      `json:"spec_task"`
}

type SpecTaskDesignReviewCommentListResponse struct {
	Comments []SpecTaskDesignReviewComment `json:"comments"`
	Total    int                           `json:"total"`
}

type SpecTaskGitPushEventListResponse struct {
	Events []SpecTaskGitPushEvent `json:"events"`
	Total  int                    `json:"total"`
}

// Implementation phase transition response
type SpecTaskImplementationStartResponse struct {
	BranchName        string `json:"branch_name"`
	BaseBranch        string `json:"base_branch"`
	RepositoryID      string `json:"repository_id"`
	RepositoryName    string `json:"repository_name"`
	LocalPath         string `json:"local_path"`
	Status            string `json:"status"`
	AgentInstructions string `json:"agent_instructions"`
	PRTemplateURL     string `json:"pr_template_url,omitempty"`
	CreatedAt         string `json:"created_at"`
}

// GORM Hooks

func (r *SpecTaskDesignReview) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = GenerateSpecTaskDesignReviewID()
	}
	return nil
}

func (r *SpecTaskDesignReview) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedAt = time.Now()
	return nil
}

func (c *SpecTaskDesignReviewComment) BeforeCreate(tx *gorm.DB) error {
	if c.ID == "" {
		c.ID = GenerateSpecTaskDesignReviewCommentID()
	}
	return nil
}

func (c *SpecTaskDesignReviewComment) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedAt = time.Now()
	return nil
}

func (r *SpecTaskDesignReviewCommentReply) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = GenerateSpecTaskDesignReviewCommentReplyID()
	}
	return nil
}

func (e *SpecTaskGitPushEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == "" {
		e.ID = GenerateSpecTaskGitPushEventID()
	}
	return nil
}

// Table names

func (SpecTaskDesignReview) TableName() string {
	return "spec_task_design_reviews"
}

func (SpecTaskDesignReviewComment) TableName() string {
	return "spec_task_design_review_comments"
}

func (SpecTaskDesignReviewCommentReply) TableName() string {
	return "spec_task_design_review_comment_replies"
}

func (SpecTaskGitPushEvent) TableName() string {
	return "spec_task_git_push_events"
}

// ID generators

func GenerateSpecTaskDesignReviewID() string {
	return "stdr_" + generateUUID()
}

func GenerateSpecTaskDesignReviewCommentID() string {
	return "stdrc_" + generateUUID()
}

func GenerateSpecTaskDesignReviewCommentReplyID() string {
	return "stdrcr_" + generateUUID()
}

func GenerateSpecTaskGitPushEventID() string {
	return "stgpe_" + generateUUID()
}

// Helper methods

func (r *SpecTaskDesignReview) IsPending() bool {
	return r.Status == SpecTaskDesignReviewStatusPending
}

func (r *SpecTaskDesignReview) IsInReview() bool {
	return r.Status == SpecTaskDesignReviewStatusInReview
}

func (r *SpecTaskDesignReview) IsApproved() bool {
	return r.Status == SpecTaskDesignReviewStatusApproved
}

func (r *SpecTaskDesignReview) ChangesRequested() bool {
	return r.Status == SpecTaskDesignReviewStatusChangesRequested
}

func (r *SpecTaskDesignReview) IsSuperseded() bool {
	return r.Status == SpecTaskDesignReviewStatusSuperseded
}

func (c *SpecTaskDesignReviewComment) IsResolved() bool {
	return c.Resolved
}

func (c *SpecTaskDesignReviewComment) IsCritical() bool {
	return c.CommentType == SpecTaskDesignReviewCommentTypeCritical
}

func (c *SpecTaskDesignReviewComment) IsQuestion() bool {
	return c.CommentType == SpecTaskDesignReviewCommentTypeQuestion
}

func (e *SpecTaskGitPushEvent) IsProcessed() bool {
	return e.Processed
}

func (e *SpecTaskGitPushEvent) HasError() bool {
	return e.ProcessingError != ""
}
