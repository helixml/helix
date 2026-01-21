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
	ID             string    `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Name           string    `gorm:"index" json:"name"`
	Description    string    `json:"description"`
	OwnerID        string    `gorm:"index" json:"owner_id"`
	OrganizationID string    `gorm:"index" json:"organization_id"` // Organization ID - will be backfilled for existing repos
	// Deprecated: ProjectID is maintained for backward compatibility only.
	// Use the project_repositories junction table for many-to-many project-repo relationships.
	// This column is kept in the database for rollback compatibility but reads should use the junction table.
	ProjectID     string                 `gorm:"index" json:"project_id"`
	RepoType      GitRepositoryType      `gorm:"index" json:"repo_type"`
	Status        GitRepositoryStatus    `json:"status"`
	CloneURL      string                 `json:"clone_url"`  // For Helix-hosted: http://api/git/{repo_id}, For external: https://github.com/org/repo.git
	LocalPath     string                 `json:"local_path"` // Local filesystem path for Helix-hosted repos (empty for external)
	DefaultBranch string                 `json:"default_branch"`
	Branches      []string               `json:"branches" gorm:"type:jsonb;serializer:json"`
	LastActivity  time.Time              `json:"last_activity" gorm:"index"`
	Metadata      map[string]interface{} `gorm:"type:jsonb;serializer:json" json:"metadata"` // Stores Metadata as JSON

	// External repository fields
	IsExternal   bool                   `gorm:"index" json:"is_external"` // True for GitHub/GitLab/ADO, false for Helix-hosted
	ExternalURL  string                 `json:"external_url"`             // Full URL to external repo (e.g., https://github.com/org/repo)
	ExternalType ExternalRepositoryType `json:"external_type"`            // "github", "gitlab", "ado", "bitbucket", etc.

	// Authentication fields
	Username string `json:"username"` // Username for the repository
	Password string `json:"password"` // Password for the repository

	// Provider-specific settings
	AzureDevOps *AzureDevOps `gorm:"type:jsonb;serializer:json" json:"azure_devops"`
	GitHub      *GitHub      `gorm:"type:jsonb;serializer:json" json:"github"`
	GitLab      *GitLab      `gorm:"type:jsonb;serializer:json" json:"gitlab"`
	Bitbucket   *Bitbucket   `gorm:"type:jsonb;serializer:json" json:"bitbucket"`

	// OAuth connection ID - references an OAuthConnection for authentication
	// When set, uses the OAuth access token instead of username/password or PAT
	OAuthConnectionID string `gorm:"index" json:"oauth_connection_id"`

	// TODO: SSH key support

	// Code intelligence fields
	KoditIndexing bool `gorm:"index" json:"kodit_indexing"` // Enable Kodit indexing for code intelligence (MCP server for snippets/architecture)
}

type AzureDevOps struct {
	OrganizationURL     string `json:"organization_url"`
	PersonalAccessToken string `json:"personal_access_token"`

	// Service Principal authentication (service-to-service via Azure AD/Entra ID)
	// Uses OAuth 2.0 client credentials flow for automated system access
	TenantID     string `json:"tenant_id,omitempty"`     // Azure AD tenant ID
	ClientID     string `json:"client_id,omitempty"`     // App registration client ID
	ClientSecret string `json:"client_secret,omitempty"` // App registration client secret
}

// GitHub contains GitHub-specific authentication settings
type GitHub struct {
	PersonalAccessToken string `json:"personal_access_token"`
	BaseURL             string `json:"base_url"` // For GitHub Enterprise instances (empty for github.com)

	// GitHub App authentication (service-to-service)
	// When AppID and PrivateKey are set, uses GitHub App installation tokens
	AppID          int64  `json:"app_id,omitempty"`          // GitHub App ID
	InstallationID int64  `json:"installation_id,omitempty"` // Installation ID for the app on the org/repo
	PrivateKey     string `json:"private_key,omitempty"`     // PEM-encoded private key for JWT signing
}

// GitLab contains GitLab-specific authentication settings
type GitLab struct {
	PersonalAccessToken string `json:"personal_access_token"`
	BaseURL             string `json:"base_url"` // For self-hosted GitLab instances (empty for gitlab.com)
}

// Bitbucket contains Bitbucket-specific authentication settings
type Bitbucket struct {
	Username    string `json:"username"`     // Bitbucket username (required for API auth)
	AppPassword string `json:"app_password"` // Bitbucket App Password (recommended over regular password)
	BaseURL     string `json:"base_url"`     // For Bitbucket Server/Data Center (empty for bitbucket.org)
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

	// Provider-specific settings
	AzureDevOps *AzureDevOps `json:"azure_devops,omitempty"`
	GitHub      *GitHub      `json:"github,omitempty"`
	GitLab      *GitLab      `json:"gitlab,omitempty"`
	Bitbucket   *Bitbucket   `json:"bitbucket,omitempty"`

	// OAuth connection ID - references an OAuthConnection for authentication
	OAuthConnectionID string `json:"oauth_connection_id,omitempty"`

	KoditIndexing bool `json:"kodit_indexing"` // Enable Kodit code intelligence indexing

	// Internal fields - not exposed in API
	// Set by handler when user authenticates with API key, used for Kodit to clone local repos
	KoditAPIKey string `json:"-"`
	// Set by handler from authenticated user - used for git commits
	// Enterprise ADO deployments reject commits with non-corporate email addresses
	CreatorName  string `json:"-"`
	CreatorEmail string `json:"-"`
}

// GitRepositoryUpdateRequest represents a request to update a repository
type GitRepositoryUpdateRequest struct {
	Name              string                 `json:"name,omitempty"`
	Description       string                 `json:"description,omitempty"`
	DefaultBranch     string                 `json:"default_branch,omitempty"`
	Username          string                 `json:"username,omitempty"`
	Password          string                 `json:"password,omitempty"`
	ExternalURL       string                 `json:"external_url,omitempty"`
	ExternalType      ExternalRepositoryType `json:"external_type"` // "github", "gitlab", "ado", "bitbucket", etc.
	AzureDevOps       *AzureDevOps           `json:"azure_devops,omitempty"`
	GitHub            *GitHub                `json:"github,omitempty"`
	GitLab            *GitLab                `json:"gitlab,omitempty"`
	Bitbucket         *Bitbucket             `json:"bitbucket,omitempty"`
	OAuthConnectionID *string                `json:"oauth_connection_id,omitempty"` // OAuth connection for authentication
	Metadata          map[string]interface{} `json:"metadata,omitempty"`
	KoditIndexing     *bool                  `json:"kodit_indexing,omitempty"` // Enable Kodit code intelligence indexing (pointer to distinguish unset from false)
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
	// Internal fields - set by handler from authenticated user
	// Enterprise ADO deployments reject commits with non-corporate email addresses
	CreatorName  string `json:"-"`
	CreatorEmail string `json:"-"`
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
	Commits        []*Commit      `json:"commits"`
	ExternalStatus ExternalStatus `json:"external_status"`
	// Pagination info
	Total   int `json:"total"`
	Page    int `json:"page"`
	PerPage int `json:"per_page"`
}

type ExternalStatus struct {
	CommitsAhead  int `json:"commits_ahead"`
	CommitsBehind int `json:"commits_behind"`
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

type PullRequest struct {
	ID           string           `json:"id"`
	Number       int              `json:"number"`
	Title        string           `json:"title"`
	Description  string           `json:"description"`
	State        PullRequestState `json:"state"`
	SourceBranch string           `json:"source_branch"`
	TargetBranch string           `json:"target_branch"`
	Author       string           `json:"author"`
	CreatedAt    time.Time        `json:"created_at"`
	UpdatedAt    time.Time        `json:"updated_at"`
	URL          string           `json:"url,omitempty"`
}

type PullRequestState string

const (
	PullRequestStateOpen    PullRequestState = "open"
	PullRequestStateClosed  PullRequestState = "closed"
	PullRequestStateMerged  PullRequestState = "merged"
	PullRequestStateUnknown PullRequestState = "unknown"
)

type CreatePullRequestRequest struct {
	Title        string `json:"title"`
	Description  string `json:"description"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
}

type CreatePullRequestResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

type PullResponse struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
}

type PushResponse struct {
	RepositoryID string `json:"repository_id"`
	Branch       string `json:"branch"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
}

type SyncAllResponse struct {
	RepositoryID string `json:"repository_id"`
	Success      bool   `json:"success"`
	Message      string `json:"message"`
}
