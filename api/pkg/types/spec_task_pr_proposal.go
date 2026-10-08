package types

import "time"

// SpecTaskPRProposalStatus is the lifecycle of an agent's request to open a
// pull request.
//
//	pending  → rejected                      (user said no; no push rights granted)
//	pending  → approved → opened             (branch pushed upstream, PR exists)
//	                    ↘ failed → approved   (user retries after fixing the cause)
//
// approved means the user granted push rights to HeadBranch but the PR could not
// be opened yet, usually because the branch has no commits ahead of the base.
// The next push to HeadBranch (or the orchestrator's retry) opens it.
type SpecTaskPRProposalStatus string

const (
	PRProposalStatusPending  SpecTaskPRProposalStatus = "pending"
	PRProposalStatusApproved SpecTaskPRProposalStatus = "approved"
	PRProposalStatusOpened   SpecTaskPRProposalStatus = "opened"
	PRProposalStatusRejected SpecTaskPRProposalStatus = "rejected"
	PRProposalStatusFailed   SpecTaskPRProposalStatus = "failed"
)

// SpecTaskPRProposal is a spec task agent's request to push a branch and open a
// pull request from it. Every PR a spec task opens goes through one: the user
// approves the exact branch name, base, title and body before anything reaches
// the external provider.
type SpecTaskPRProposal struct {
	ID         string `json:"id" gorm:"primaryKey;size:255"`
	SpecTaskID string `json:"spec_task_id" gorm:"not null;size:255;index"`
	ProjectID  string `json:"project_id" gorm:"not null;size:255;index"`

	RepositoryID   string `json:"repository_id" gorm:"not null;size:255"`
	RepositoryName string `json:"repository_name" gorm:"size:255"`
	HeadBranch     string `json:"head_branch" gorm:"not null;size:255"`
	BaseBranch     string `json:"base_branch" gorm:"not null;size:255"`
	Title          string `json:"title" gorm:"type:text"`
	Body           string `json:"body" gorm:"type:text"`
	Reason         string `json:"reason" gorm:"type:text"`

	Status            SpecTaskPRProposalStatus `json:"status" gorm:"not null;size:32;index"`
	ProposedBySession string                   `json:"proposed_by_session,omitempty" gorm:"size:255"`

	DecidedBy       string     `json:"decided_by,omitempty" gorm:"size:255"`
	DecidedAt       *time.Time `json:"decided_at,omitempty"`
	DecisionComment string     `json:"decision_comment,omitempty" gorm:"type:text"`
	AutoApproved    bool       `json:"auto_approved,omitempty"`

	PRID     string `json:"pr_id,omitempty" gorm:"size:255"`
	PRNumber int    `json:"pr_number,omitempty"`
	PRURL    string `json:"pr_url,omitempty" gorm:"type:text"`
	Error    string `json:"error,omitempty" gorm:"type:text"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GrantsPush reports whether the proposal gives the agent push rights to its
// head branch: once a user approves it, until it is rejected.
func (p *SpecTaskPRProposal) GrantsPush() bool {
	switch p.Status {
	case PRProposalStatusApproved, PRProposalStatusOpened, PRProposalStatusFailed:
		return true
	}
	return false
}

// AwaitsPR reports whether the proposal still expects a PR to be opened; while
// any proposal does, the task must not be marked done.
func (p *SpecTaskPRProposal) AwaitsPR() bool {
	switch p.Status {
	case PRProposalStatusPending, PRProposalStatusApproved, PRProposalStatusFailed:
		return true
	}
	return false
}

// SpecTaskPRProposalFilter selects proposals. Empty fields are not filtered on.
type SpecTaskPRProposalFilter struct {
	SpecTaskID   string
	ProjectID    string
	RepositoryID string
	HeadBranch   string
	Statuses     []SpecTaskPRProposalStatus
}

// PRProposalDecisionRequest is the user's decision on a pending (or failed)
// proposal. Edited fields override the agent's proposal before the PR opens.
type PRProposalDecisionRequest struct {
	Decision   string `json:"decision"` // "approve" or "reject"
	Comment    string `json:"comment,omitempty"`
	HeadBranch string `json:"head_branch,omitempty"`
	BaseBranch string `json:"base_branch,omitempty"`
	Title      string `json:"title,omitempty"`
	Body       string `json:"body,omitempty"`
	// AutoApproveFuture, with an approval, approves this task's later
	// proposals without asking, as the deciding user.
	AutoApproveFuture bool `json:"auto_approve_future,omitempty"`
}

const (
	PRProposalDecisionApprove = "approve"
	PRProposalDecisionReject  = "reject"
)

// InitAutoApprovePullRequests sets a new task's auto-approve setting: the
// explicit request if given, else the project's default. actorID is the user
// creating the task, whose provider credentials auto-approval will use.
func (t *SpecTask) InitAutoApprovePullRequests(requested *bool, project *Project, actorID string) {
	enabled := project != nil && project.AutoApprovePullRequests
	if requested != nil {
		enabled = *requested
	}
	t.AutoApprovePullRequests = enabled
	if enabled {
		t.AutoApprovePullRequestsBy = actorID
	}
}
