package types

// RepositoryInfo represents a repository from an external provider (GitHub, GitLab, etc.)
// Used for listing repositories available to browse/attach from OAuth connections
type RepositoryInfo struct {
	Name        string `json:"name"`
	FullName    string `json:"full_name"`    // e.g., "owner/repo" for GitHub or "group/project" for GitLab
	CloneURL    string `json:"clone_url"`    // HTTPS clone URL
	HTMLURL     string `json:"html_url"`     // Web URL to view the repository
	Description string `json:"description"`
	Private     bool   `json:"private"`
	DefaultBranch string `json:"default_branch,omitempty"`
}

// ListOAuthRepositoriesResponse is the response for listing repositories from an OAuth connection
type ListOAuthRepositoriesResponse struct {
	Repositories []RepositoryInfo `json:"repositories"`
}
