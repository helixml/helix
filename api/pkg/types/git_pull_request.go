package types

import (
	"fmt"
	"time"
)

// GitPullRequestState is the workflow state of an internal pull request.
type GitPullRequestState string

const (
	GitPullRequestStateOpen   GitPullRequestState = "open"
	GitPullRequestStateClosed GitPullRequestState = "closed"
)

// InternalPRFile is one changed file of an internal pull request, rendered in
// the shape of GitHub's pull-request files endpoint.
type InternalPRFile struct {
	Filename         string `json:"filename"`
	PreviousFilename string `json:"previous_filename,omitempty"`
	Status           string `json:"status"` // added | modified | removed | renamed
	Additions        int    `json:"additions"`
	Deletions        int    `json:"deletions"`
	Changes          int    `json:"changes"`
	Patch            string `json:"patch,omitempty"`
}

// GitPullRequest is a pull request on a Helix-hosted repository. It is served
// through Helix's GitHub-compatible API (/api/github/api/v3) so the same
// go-github code path serves GitHub and Helix-hosted repos.
type GitPullRequest struct {
	ID           string              `gorm:"primaryKey" json:"id"`
	CreatedAt    time.Time           `json:"created_at"`
	UpdatedAt    time.Time           `json:"updated_at"`
	RepositoryID string              `gorm:"not null;uniqueIndex:idx_git_pull_requests_repo_number,priority:1" json:"repository_id"`
	Number       int                 `gorm:"not null;uniqueIndex:idx_git_pull_requests_repo_number,priority:2" json:"number"`
	Title        string              `json:"title"`
	Body         string              `json:"body"`
	HeadBranch   string              `gorm:"not null" json:"head_branch"`
	BaseBranch   string              `gorm:"not null" json:"base_branch"`
	HeadSHA      string              `json:"head_sha"`
	BaseSHA      string              `json:"base_sha"`
	State        GitPullRequestState `gorm:"not null;default:open" json:"state"`
	Merged       bool                `json:"merged"`
	MergedAt     *time.Time          `json:"merged_at,omitempty"`
	MergedBy     string              `json:"merged_by,omitempty"`
	MergeCommit  string              `json:"merge_commit_sha,omitempty"`
	AuthorID     string              `json:"author_id"`
	AuthorLogin  string              `json:"author_login,omitempty"`
	ClosedAt     *time.Time          `json:"closed_at,omitempty"`
}

// TableName overrides the table name
func (GitPullRequest) TableName() string {
	return "git_pull_requests"
}

func (p *GitPullRequest) String() string {
	return fmt.Sprintf("%s#%d", p.RepositoryID, p.Number)
}

// GitPullRequestDetail is the Helix UI's view of an internal pull request.
type GitPullRequestDetail struct {
	PullRequest *GitPullRequest   `json:"pull_request"`
	HTMLURL     string            `json:"html_url"`
	Mergeable   *bool             `json:"mergeable,omitempty"`
	Files       []*InternalPRFile `json:"files"`
	// The spec task whose work this pull request carries, if any.
	SpecTaskID        string `json:"spec_task_id,omitempty"`
	SpecTaskName      string `json:"spec_task_name,omitempty"`
	SpecTaskProjectID string `json:"spec_task_project_id,omitempty"`
}

// GitPullRequestUpdate edits an internal pull request. Nil fields are left as is.
type GitPullRequestUpdate struct {
	Title *string `json:"title,omitempty"`
	Body  *string `json:"body,omitempty"`
	State *string `json:"state,omitempty"` // open | closed
}

// GitPullRequestMergeRequest merges an internal pull request.
type GitPullRequestMergeRequest struct {
	MergeMethod   string `json:"merge_method,omitempty"` // merge (the only supported method)
	CommitTitle   string `json:"commit_title,omitempty"`
	CommitMessage string `json:"commit_message,omitempty"`
}
