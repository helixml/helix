package types

import (
	"time"
)

// GitRepositoryType defines the type of repository
type GitRepositoryType string

const (
	GitRepositoryTypeInternal GitRepositoryType = "internal" // Internal project config repository
	GitRepositoryTypeCode     GitRepositoryType = "code"     // Code repository (user projects, samples, external repos)
)

// GitRepositoryStatus defines the status of a repository
type GitRepositoryStatus string

const (
	GitRepositoryStatusActive   GitRepositoryStatus = "active"
	GitRepositoryStatusArchived GitRepositoryStatus = "archived"
	GitRepositoryStatusDeleted  GitRepositoryStatus = "deleted"
)

// GitRepository represents a git repository
// Supports both Helix-hosted repositories and external repositories (GitHub, GitLab, ADO, etc.)
type GitRepository struct {
	ID             string                 `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time              `json:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at"`
	Name           string                 `gorm:"index" json:"name"`
	Description    string                 `json:"description"`
	OwnerID        string                 `gorm:"index" json:"owner_id"`
	OrganizationID string                 `gorm:"index" json:"organization_id"` // Organization ID - will be backfilled for existing repos
	ProjectID      string                 `gorm:"index" json:"project_id"`
	RepoType       GitRepositoryType      `gorm:"index" json:"repo_type"`
	Status         GitRepositoryStatus    `json:"status"`
	CloneURL       string                 `json:"clone_url"`  // For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git
	LocalPath      string                 `json:"local_path"` // Local filesystem path for Helix-hosted repos (empty for external)
	DefaultBranch  string                 `json:"default_branch"`
	Branches       []string               `json:"branches" gorm:"type:jsonb;serializer:json"`
	LastActivity   time.Time              `json:"last_activity" gorm:"index"`
	Metadata       map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"` // Stores Metadata as JSON

	// External repository fields
	IsExternal   bool                   `gorm:"index" json:"is_external"` // True for GitHub/GitLab/ADO, false for Helix-hosted
	ExternalURL  string                 `json:"external_url"`             // Full URL to external repo (e.g., https://github.com/org/repo)
	ExternalType ExternalRepositoryType `json:"external_type"`            // "github", "gitlab", "ado", "bitbucket", etc.

	// Authentication fields
	Username string `json:"username"` // Username for the repository
	Password string `json:"password"` // Password for the repository

	// TODO: OAuth support using our providers
	// TODO: SSH key

	AzureDevOps *AzureDevOps `gorm:"type:jsonb;serializer:json" json:"azure_devops"`

	// Code intelligence fields
	KoditIndexing bool `gorm:"index" json:"kodit_indexing"` // Enable Kodit indexing for code intelligence (MCP server for snippets/architecture)
}

type AzureDevOps struct {
	OrganizationURL     string `json:"organization_url"`
	PersonalAccessToken string `json:"personal_access_token"`
}

// TableName overrides the table name
func (GitRepository) TableName() string {
	return "git_repositories"
}

type ExternalRepositoryType string

const (
	ExternalRepositoryTypeGitHub    ExternalRepositoryType = "github"
	ExternalRepositoryTypeGitLab    ExternalRepositoryType = "gitlab"
	ExternalRepositoryTypeADO       ExternalRepositoryType = "ado"
	ExternalRepositoryTypeBitbucket ExternalRepositoryType = "bitbucket"
)

// GitRepositoryCreateRequest represents a request to create a new repository
type GitRepositoryCreateRequest struct {
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	RepoType       GitRepositoryType      `json:"repo_type"`
	OwnerID        string                 `json:"owner_id"`
	OrganizationID string                 `json:"organization_id,omitempty"` // Organization ID - required for access control
	ProjectID      string                 `json:"project_id,omitempty"`
	InitialFiles   map[string]string      `json:"initial_files,omitempty"`
	DefaultBranch  string                 `json:"default_branch,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`

	IsExternal   bool                   `json:"is_external"`   // True for GitHub/GitLab/ADO, false for Helix-hosted
	ExternalURL  string                 `json:"external_url"`  // Full URL to external repo (e.g., https://github.com/org/repo)
	ExternalType ExternalRepositoryType `json:"external_type"` // "github", "gitlab", "ado", "bitbucket", etc.

	Username string `json:"username"` // Username for the repository
	Password string `json:"password"` // Password for the repository

	AzureDevopsRepository *AzureDevOps `json:"azure_devops_repository"`

	KoditIndexing bool `json:"kodit_indexing"` // Enable Kodit code intelligence indexing
}

// GitRepositoryUpdateRequest represents a request to update a repository
type GitRepositoryUpdateRequest struct {
	Name                  string                 `json:"name,omitempty"`
	Description           string                 `json:"description,omitempty"`
	DefaultBranch         string                 `json:"default_branch,omitempty"`
	Username              string                 `json:"username,omitempty"`
	Password              string                 `json:"password,omitempty"`
	ExternalURL           string                 `json:"external_url,omitempty"`
	AzureDevopsRepository *AzureDevOps           `json:"azure_devops_repository"`
	Metadata              map[string]interface{} `json:"metadata,omitempty"`
}

type ListGitRepositoriesRequest struct {
	OrganizationID string
	OwnerID        string
	ProjectID      string
}

// CreateSampleRepositoryRequest represents a request to create a sample repository
type CreateSampleRepositoryRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	OwnerID        string `json:"owner_id"`
	OrganizationID string `json:"organization_id"`
	SampleType     string `json:"sample_type"`
	KoditIndexing  bool   `json:"kodit_indexing"` // Enable Kodit code intelligence indexing
}

// TreeEntry represents a file or directory in a repository
type TreeEntry struct {
	Name  string `json:"name"`
	Path  string `json:"path"`
	IsDir bool   `json:"is_dir"`
	Size  int64  `json:"size"`
}

// GitRepositoryTreeResponse represents the response for browsing repository tree
type GitRepositoryTreeResponse struct {
	Path    string      `json:"path"`
	Entries []TreeEntry `json:"entries"`
}

// GitRepositoryFileResponse represents the response for getting file contents
type GitRepositoryFileResponse struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type UpdateGitRepositoryFileContentsRequest struct {
	Path    string `json:"path"`    // File path
	Branch  string `json:"branch"`  // Branch name
	Message string `json:"message"` // Commit message
	Author  string `json:"author"`  // Author name
	Email   string `json:"email"`   // Author email
	Content string `json:"content"` // Base64 encoded content
}

type ListCommitsRequest struct {
	RepoID  string `json:"repo_id"`
	Branch  string `json:"branch"`
	Since   string `json:"since"`
	Until   string `json:"until"`
	PerPage int    `json:"per_page"`
	Page    int    `json:"page"`
}

type ListCommitsResponse struct {
	Commits []*Commit `json:"commits"`
}

type Commit struct {
	SHA       string    `json:"sha"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Email     string    `json:"email"`
	Timestamp time.Time `json:"timestamp"`
}

type CreateBranchRequest struct {
	BranchName string `json:"branch_name"`
	BaseBranch string `json:"base_branch,omitempty"`
}

type CreateBranchResponse struct {
	RepositoryID string `json:"repository_id"`
	BranchName   string `json:"branch_name"`
	BaseBranch   string `json:"base_branch"`
	Message      string `json:"message"`
}
