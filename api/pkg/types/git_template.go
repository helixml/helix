package types

// GitOwnerKind is the kind of account a repository can be created under.
type GitOwnerKind string

const (
	GitOwnerKindUser  GitOwnerKind = "user"
	GitOwnerKindOrg   GitOwnerKind = "org"
	GitOwnerKindGroup GitOwnerKind = "group"
)

// GitOwner is an account (user, GitHub org or GitLab group) the acting user
// can create repositories under.
type GitOwner struct {
	// Login is the owner to pass to CreateRepoFromTemplateRequest.Owner: the
	// GitHub login/org name, or the GitLab user path / group full path.
	Login     string       `json:"login"`
	Kind      GitOwnerKind `json:"kind"`
	AvatarURL string       `json:"avatar_url"`
	// ID is the GitHub account id, or the GitLab namespace id.
	ID int64 `json:"id"`
}

// CreateRepoFromTemplateRequest creates a repository in the user's own
// GitHub/GitLab account from a template repository on github.com.
type CreateRepoFromTemplateRequest struct {
	Provider ExternalRepositoryType `json:"provider"`
	Owner    string                 `json:"owner"`
	Name     string                 `json:"name"`
	// Private defaults to true when omitted.
	Private     *bool  `json:"private,omitempty"`
	Description string `json:"description,omitempty"`
	// Template is the github.com template repository as "owner/repo".
	Template string `json:"template"`
}

// CreateRepoFromTemplateResponse describes the created repository.
type CreateRepoFromTemplateResponse struct {
	CloneURL      string                 `json:"clone_url"`
	HTMLURL       string                 `json:"html_url"`
	DefaultBranch string                 `json:"default_branch"`
	Provider      ExternalRepositoryType `json:"provider"`
	FullName      string                 `json:"full_name"`
}

// ConnectRequiredErrorCode is the ConnectRequiredError.Code value.
const ConnectRequiredErrorCode = "connect_required"

// ConnectRequiredError is the 409 body returned when the user has no usable
// credential for the git provider. ConnectURL opens the Helix page where the
// user connects the provider.
type ConnectRequiredError struct {
	Code       string                 `json:"code"`
	Provider   ExternalRepositoryType `json:"provider"`
	ConnectURL string                 `json:"connect_url"`
}
