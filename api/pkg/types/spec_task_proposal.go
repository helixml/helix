package types

import (
	"time"

	"gorm.io/datatypes"
)

// SpecTaskProposalKind discriminates between the three proposal kinds an agent can make.
type SpecTaskProposalKind string

const (
	// ProposalKindPullRequest: agent wants to open a PR from a branch (possibly non-default).
	ProposalKindPullRequest SpecTaskProposalKind = "pull_request"
	// ProposalKindSpecTask: agent wants to spawn a follow-up spec task in this project.
	ProposalKindSpecTask SpecTaskProposalKind = "spec_task"
	// ProposalKindMarkComplete: agent declares its task is finished; user confirms or sends back.
	ProposalKindMarkComplete SpecTaskProposalKind = "mark_complete"
)

// SpecTaskProposalStatus is the lifecycle of a proposal.
type SpecTaskProposalStatus string

const (
	ProposalStatusPending  SpecTaskProposalStatus = "pending"
	ProposalStatusApproved SpecTaskProposalStatus = "approved"
	ProposalStatusRejected SpecTaskProposalStatus = "rejected"
	// ProposalStatusFailed: user approved but the resulting action (PR open / task create) failed.
	ProposalStatusFailed SpecTaskProposalStatus = "failed"
)

// SpecTaskProposal is a unified record for the three kinds of agent-proposed actions
// that require user approval in the Helix UI before execution. Kind discriminates
// which payload columns are populated; the rest are nullable/empty.
type SpecTaskProposal struct {
	ID         string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`
	ProjectID  string `json:"project_id" gorm:"not null;size:255;index"`

	Kind   SpecTaskProposalKind   `json:"kind" gorm:"not null;size:50;index"`
	Status SpecTaskProposalStatus `json:"status" gorm:"not null;size:50;default:pending;index"`

	// Created-by-agent metadata
	ProposedBySession string `json:"proposed_by_session,omitempty" gorm:"size:255"` // session_id
	AgentReason       string `json:"agent_reason,omitempty" gorm:"type:text"`       // free-form why-we-want-this

	// PR proposal payload (kind = ProposalKindPullRequest)
	PRRepositoryID string `json:"pr_repository_id,omitempty" gorm:"size:255"`
	PRHeadBranch   string `json:"pr_head_branch,omitempty" gorm:"size:255"`
	PRBaseBranch   string `json:"pr_base_branch,omitempty" gorm:"size:255"`
	PRTitle        string `json:"pr_title,omitempty" gorm:"type:text"`
	PRBody         string `json:"pr_body,omitempty" gorm:"type:text"`

	// Spec task proposal payload (kind = ProposalKindSpecTask)
	TaskName           string           `json:"task_name,omitempty" gorm:"size:500"`
	TaskDescription    string           `json:"task_description,omitempty" gorm:"type:text"`
	TaskType           string           `json:"task_type,omitempty" gorm:"size:50"`
	TaskPriority       SpecTaskPriority `json:"task_priority,omitempty" gorm:"size:50"`
	TaskOriginalPrompt string           `json:"task_original_prompt,omitempty" gorm:"type:text"`

	// Mark-complete payload (kind = ProposalKindMarkComplete)
	CompleteReason string `json:"complete_reason,omitempty" gorm:"type:text"`

	// Decision tracking
	DecidedBy       string         `json:"decided_by,omitempty" gorm:"size:255;index"` // user ID who approved/rejected
	DecidedAt       *time.Time     `json:"decided_at,omitempty"`
	DecisionComment string         `json:"decision_comment,omitempty" gorm:"type:text"`
	EditedPayload   datatypes.JSON `json:"edited_payload,omitempty" gorm:"type:jsonb"` // user edits to the payload, if any

	// Result tracking — what actually happened on approval
	ResultPRID   string `json:"result_pr_id,omitempty" gorm:"size:255"`   // for PR kind
	ResultPRURL  string `json:"result_pr_url,omitempty" gorm:"size:1024"` // for PR kind
	ResultTaskID string `json:"result_task_id,omitempty" gorm:"size:255"` // for spec_task kind
	ResultError  string `json:"result_error,omitempty" gorm:"type:text"`  // populated when Status=failed

	CreatedAt time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	UpdatedAt time.Time `json:"updated_at" gorm:"not null;default:CURRENT_TIMESTAMP"`
}

// TableName overrides the default plural table name to be explicit.
func (SpecTaskProposal) TableName() string {
	return "spec_task_proposals"
}

// SpecTaskProposalFilters is the filter struct for ListSpecTaskProposals.
type SpecTaskProposalFilters struct {
	SpecTaskID string                 `json:"spec_task_id,omitempty"`
	ProjectID  string                 `json:"project_id,omitempty"`
	Kind       SpecTaskProposalKind   `json:"kind,omitempty"`
	Status     SpecTaskProposalStatus `json:"status,omitempty"`
	Limit      int                    `json:"limit,omitempty"`
}

// ProposalDecisionRequest is the body of POST /api/v1/proposals/{id}/decide
type ProposalDecisionRequest struct {
	Decision      string                 `json:"decision" validate:"required,oneof=approve reject"` // "approve" or "reject"
	Comment       string                 `json:"comment,omitempty"`
	EditedPayload map[string]interface{} `json:"edited_payload,omitempty"` // optional: user edits to the proposal payload (PR head_branch / title / etc.)
}

// ProposalDecisionResponse is what the decide endpoint returns.
type ProposalDecisionResponse struct {
	Proposal *SpecTaskProposal `json:"proposal"`
	Message  string            `json:"message,omitempty"`
}
