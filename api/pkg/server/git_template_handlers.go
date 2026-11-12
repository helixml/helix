package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	gogithub "github.com/google/go-github/v57/github"
	"github.com/rs/zerolog/log"
	gogitlab "github.com/xanzy/go-gitlab"

	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/agent/skill/gitlab"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/services"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// gitTemplateConfig holds the public-host endpoints and polling settings for
// the create-from-template endpoints. NewServer uses defaultGitTemplateConfig;
// tests point the URLs at fake provider servers.
type gitTemplateConfig struct {
	// GitHubAPIURL is the github.com API base URL ("" = https://api.github.com/).
	GitHubAPIURL string
	// GitLabURL is the gitlab.com base URL ("" = https://gitlab.com). A
	// credential for a self-hosted GitLab uses that host instead.
	GitLabURL string
	// PollInterval is how often a new repository is checked for readiness.
	PollInterval time.Duration
	// ReadyTimeout bounds the wait for a new repository to become ready.
	ReadyTimeout time.Duration
}

func defaultGitTemplateConfig() gitTemplateConfig {
	return gitTemplateConfig{
		PollInterval: 2 * time.Second,
		ReadyTimeout: 2 * time.Minute,
	}
}

// gitTemplateOAuthScopes are the OAuth scopes a connection needs to create
// repositories and list owners.
var gitTemplateOAuthScopes = map[types.ExternalRepositoryType][]string{
	types.ExternalRepositoryTypeGitHub: GitHubRepoScopes,
	types.ExternalRepositoryTypeGitLab: {"api"},
}

var templateRepoPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// gitHostCredential is the token used to call the provider API for the user.
type gitHostCredential struct {
	token string
	oauth bool
	// baseURL is the provider base URL; "" means the public host.
	baseURL string
	// httpClient is a netguard client when baseURL came from a user-supplied
	// connection, nil for the public host.
	httpClient *http.Client
}

// ambiguousGitCredentialError is returned when several personal access token
// connections could serve the request and none is preferred.
type ambiguousGitCredentialError struct {
	provider types.ExternalRepositoryType
}

func (e *ambiguousGitCredentialError) Error() string {
	return fmt.Sprintf("you have more than one %s personal access token connection; remove all but one in Connected Services", e.provider)
}

// resolveGitHostCredential finds the acting user's credential for provider.
// Resolution order (documented, not a fallback):
//  1. the user's OAuth connection for the provider that has the required
//     scopes (GitHub "repo", GitLab "api");
//  2. the user's git-provider-connection personal access token for the
//     provider. For GitHub only github.com tokens qualify, because templates
//     live on github.com. For GitLab the token's base URL (self-hosted) is used.
//
// Returns (nil, nil) when the user has no usable credential.
func (s *HelixAPIServer) resolveGitHostCredential(ctx context.Context, userID string, provider types.ExternalRepositoryType) (*gitHostCredential, error) {
	oauthType := services.OAuthProviderTypeForRepo(provider)
	conns, err := s.Store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{UserID: userID})
	if err != nil {
		return nil, fmt.Errorf("failed to list OAuth connections: %w", err)
	}
	for _, conn := range conns {
		if !services.OAuthConnectionMatchesProvider(conn, oauthType) || conn.AccessToken == "" {
			continue
		}
		if len(getMissingScopes(conn.Scopes, gitTemplateOAuthScopes[provider])) > 0 {
			continue
		}
		baseURL := ""
		if provider == types.ExternalRepositoryTypeGitLab {
			baseURL = selfHostedGitLabURL(conn.Provider)
		}
		return s.newGitHostCredential(conn.AccessToken, true, baseURL), nil
	}

	pats, err := s.Store.ListGitProviderConnections(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list git provider connections: %w", err)
	}
	var match *types.GitProviderConnection
	for _, pat := range pats {
		if pat.ProviderType != provider {
			continue
		}
		if provider == types.ExternalRepositoryTypeGitHub && !isGitHubDotCom(pat.BaseURL) {
			continue
		}
		if match != nil {
			return nil, &ambiguousGitCredentialError{provider: provider}
		}
		match = pat
	}
	if match == nil {
		return nil, nil
	}
	key, err := crypto.GetEncryptionKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get encryption key: %w", err)
	}
	token, err := crypto.DecryptAES256GCM(match.Token, key)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt %s token: %w", provider, err)
	}
	baseURL := ""
	if provider == types.ExternalRepositoryTypeGitLab {
		baseURL = match.BaseURL
	}
	return s.newGitHostCredential(string(token), false, baseURL), nil
}

func (s *HelixAPIServer) newGitHostCredential(token string, oauth bool, baseURL string) *gitHostCredential {
	return &gitHostCredential{
		token:      token,
		oauth:      oauth,
		baseURL:    baseURL,
		httpClient: gitProviderHTTPClient(s.gitProviderURLPolicy(), baseURL),
	}
}

// selfHostedGitLabURL derives the GitLab base URL from an OAuth provider's
// endpoints, returning "" for gitlab.com.
func selfHostedGitLabURL(provider types.OAuthProvider) string {
	for _, raw := range []string{provider.AuthURL, provider.TokenURL, provider.UserInfoURL} {
		u, err := url.Parse(raw)
		if err != nil || u.Hostname() == "" {
			continue
		}
		if strings.EqualFold(u.Hostname(), "gitlab.com") {
			return ""
		}
		return u.Scheme + "://" + u.Host
	}
	return ""
}

func isGitHubDotCom(baseURL string) bool {
	if baseURL == "" {
		return true
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "github.com" || host == "api.github.com"
}

func (s *HelixAPIServer) gitHubTemplateClient(cred *gitHostCredential) *github.Client {
	return github.NewClientWithPATBaseURLAndHTTPClient(cred.token, s.gitTemplate.GitHubAPIURL, nil)
}

func (s *HelixAPIServer) gitLabTemplateClient(cred *gitHostCredential) (*gitlab.Client, error) {
	baseURL := cred.baseURL
	if baseURL == "" {
		baseURL = s.gitTemplate.GitLabURL
	}
	if cred.oauth {
		return gitlab.NewClientWithOAuthAndHTTPClient(baseURL, cred.token, cred.httpClient)
	}
	return gitlab.NewClientWithPATAndHTTPClient(baseURL, cred.token, cred.httpClient)
}

// gitConnectURL is the Helix page where the user connects a git provider: the
// Connected Services dialog, which opens from the dialog query parameter on
// any page.
func (s *HelixAPIServer) gitConnectURL(provider types.ExternalRepositoryType) string {
	q := url.Values{}
	q.Set("dialog", "connected-services")
	q.Set("provider", string(provider))
	return strings.TrimRight(s.Cfg.WebServer.URL, "/") + "/orgs?" + q.Encode()
}

func (s *HelixAPIServer) writeConnectRequired(w http.ResponseWriter, provider types.ExternalRepositoryType) {
	writeResponse(w, &types.ConnectRequiredError{
		Code:       types.ConnectRequiredErrorCode,
		Provider:   provider,
		ConnectURL: s.gitConnectURL(provider),
	}, http.StatusConflict)
}

// writeGitProviderError maps a provider API error to a response: 401 means
// the credential is no longer valid (409 connect_required); any other 4xx
// (name taken, invalid name, unknown owner or template, no permission) is a
// 422 carrying the provider's message; everything else is a 502.
func (s *HelixAPIServer) writeGitProviderError(w http.ResponseWriter, provider types.ExternalRepositoryType, err error) {
	status := 0
	message := err.Error()
	var ghErr *gogithub.ErrorResponse
	var glErr *gogitlab.ErrorResponse
	switch {
	case errors.As(err, &ghErr) && ghErr.Response != nil:
		status = ghErr.Response.StatusCode
		message = gitHubErrorMessage(ghErr)
	case errors.As(err, &glErr) && glErr.Response != nil:
		status = glErr.Response.StatusCode
		message = glErr.Message
	}
	switch {
	case status == http.StatusUnauthorized:
		s.writeConnectRequired(w, provider)
	case status >= 400 && status < 500:
		writeErrResponse(w, fmt.Errorf("%s: %s", provider, message), http.StatusUnprocessableEntity)
	default:
		log.Error().Err(err).Str("provider", string(provider)).Msg("git provider request failed")
		writeErrResponse(w, fmt.Errorf("%s request failed: %s", provider, message), http.StatusBadGateway)
	}
}

func gitHubErrorMessage(e *gogithub.ErrorResponse) string {
	parts := []string{e.Message}
	for _, detail := range e.Errors {
		switch {
		case detail.Message != "":
			parts = append(parts, detail.Message)
		case detail.Field != "":
			parts = append(parts, fmt.Sprintf("%s %s", detail.Field, detail.Code))
		}
	}
	return strings.Join(parts, ": ")
}

// resolveGitTemplateCredential writes the error response and returns nil when
// no credential can be used.
func (s *HelixAPIServer) resolveGitTemplateCredential(w http.ResponseWriter, r *http.Request, provider types.ExternalRepositoryType) *gitHostCredential {
	user := getRequestUser(r)
	if user == nil {
		writeErrResponse(w, errors.New("unauthorized"), http.StatusUnauthorized)
		return nil
	}
	if provider != types.ExternalRepositoryTypeGitHub && provider != types.ExternalRepositoryTypeGitLab {
		writeErrResponse(w, fmt.Errorf("provider must be github or gitlab, got %q", provider), http.StatusBadRequest)
		return nil
	}
	cred, err := s.resolveGitHostCredential(r.Context(), user.ID, provider)
	var ambiguous *ambiguousGitCredentialError
	switch {
	case errors.As(err, &ambiguous):
		writeErrResponse(w, err, http.StatusBadRequest)
		return nil
	case err != nil:
		log.Error().Err(err).Str("user_id", user.ID).Msg("failed to resolve git provider credential")
		writeErrResponse(w, err, http.StatusInternalServerError)
		return nil
	case cred == nil:
		s.writeConnectRequired(w, provider)
		return nil
	}
	return cred
}

// listGitOwners godoc
// @Summary List accounts a repository can be created under
// @Description Lists the acting user's own account plus the GitHub organizations
// @Description or GitLab groups (Developer access or above) they belong to.
// @Description Credentials: the user's OAuth connection for the provider (GitHub
// @Description needs the repo scope, GitLab api), otherwise their personal access
// @Description token connection. Returns 409 connect_required when neither exists.
// @Tags    git-repositories
// @Produce json
// @Param   provider query string true "Git provider" Enums(github, gitlab)
// @Success 200 {array} types.GitOwner
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 409 {object} types.ConnectRequiredError
// @Failure 422 {object} system.HTTPError
// @Failure 502 {object} system.HTTPError
// @Router /api/v1/git/owners [get]
// @Security BearerAuth
func (s *HelixAPIServer) listGitOwners(w http.ResponseWriter, r *http.Request) {
	provider := types.ExternalRepositoryType(r.URL.Query().Get("provider"))
	cred := s.resolveGitTemplateCredential(w, r, provider)
	if cred == nil {
		return
	}

	var owners []types.GitOwner
	var err error
	if provider == types.ExternalRepositoryTypeGitHub {
		owners, err = s.listGitHubOwners(r.Context(), cred)
	} else {
		owners, err = s.listGitLabOwners(r.Context(), cred)
	}
	if err != nil {
		s.writeGitProviderError(w, provider, err)
		return
	}
	writeResponse(w, owners, http.StatusOK)
}

func (s *HelixAPIServer) listGitHubOwners(ctx context.Context, cred *gitHostCredential) ([]types.GitOwner, error) {
	client := s.gitHubTemplateClient(cred)
	user, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return nil, err
	}
	orgs, err := client.ListUserOrganizations(ctx)
	if err != nil {
		return nil, err
	}
	owners := make([]types.GitOwner, 0, len(orgs)+1)
	owners = append(owners, types.GitOwner{
		Login:     user.GetLogin(),
		Kind:      types.GitOwnerKindUser,
		AvatarURL: user.GetAvatarURL(),
		ID:        user.GetID(),
	})
	for _, org := range orgs {
		owners = append(owners, types.GitOwner{
			Login:     org.GetLogin(),
			Kind:      types.GitOwnerKindOrg,
			AvatarURL: org.GetAvatarURL(),
			ID:        org.GetID(),
		})
	}
	return owners, nil
}

func (s *HelixAPIServer) listGitLabOwners(ctx context.Context, cred *gitHostCredential) ([]types.GitOwner, error) {
	client, err := s.gitLabTemplateClient(cred)
	if err != nil {
		return nil, err
	}
	user, err := client.GetCurrentUser(ctx)
	if err != nil {
		return nil, err
	}
	userNamespace, err := client.GetNamespace(ctx, user.Username)
	if err != nil {
		return nil, err
	}
	groups, err := client.ListGroupsWithMinAccess(ctx, gogitlab.DeveloperPermissions)
	if err != nil {
		return nil, err
	}
	owners := make([]types.GitOwner, 0, len(groups)+1)
	owners = append(owners, types.GitOwner{
		Login:     userNamespace.FullPath,
		Kind:      types.GitOwnerKindUser,
		AvatarURL: user.AvatarURL,
		ID:        int64(userNamespace.ID),
	})
	for _, group := range groups {
		owners = append(owners, types.GitOwner{
			Login:     group.FullPath,
			Kind:      types.GitOwnerKindGroup,
			AvatarURL: group.AvatarURL,
			ID:        int64(group.ID),
		})
	}
	return owners, nil
}

// createGitRepositoryFromTemplate godoc
// @Summary Create a repository from a template in the user's GitHub/GitLab account
// @Description Creates a repository under owner using the acting user's own credentials.
// @Description GitHub generates it from the template repository (the template must have
// @Description the template flag); GitLab imports https://github.com/<template>.git and
// @Description the call waits for the import to finish. private defaults to true.
// @Description Credentials: the user's OAuth connection for the provider (GitHub needs
// @Description the repo scope, GitLab api), otherwise their personal access token
// @Description connection. Returns 409 connect_required when neither exists, and 422
// @Description with the provider's message when the provider rejects the request
// @Description (for example, the name is already taken).
// @Tags    git-repositories
// @Accept  json
// @Produce json
// @Param   request body types.CreateRepoFromTemplateRequest true "Repository to create"
// @Success 201 {object} types.CreateRepoFromTemplateResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 409 {object} types.ConnectRequiredError
// @Failure 422 {object} system.HTTPError
// @Failure 502 {object} system.HTTPError
// @Failure 504 {object} system.HTTPError
// @Router /api/v1/git/repositories/from-template [post]
// @Security BearerAuth
func (s *HelixAPIServer) createGitRepositoryFromTemplate(w http.ResponseWriter, r *http.Request) {
	var req types.CreateRepoFromTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrResponse(w, jsonDecodeHTTPError(err), http.StatusBadRequest)
		return
	}
	if err := normalizeCreateRepoFromTemplate(&req); err != nil {
		writeErrResponse(w, err, http.StatusBadRequest)
		return
	}
	cred := s.resolveGitTemplateCredential(w, r, req.Provider)
	if cred == nil {
		return
	}

	var resp *types.CreateRepoFromTemplateResponse
	var err error
	if req.Provider == types.ExternalRepositoryTypeGitHub {
		resp, err = s.createGitHubRepoFromTemplate(r.Context(), cred, &req)
	} else {
		resp, err = s.createGitLabRepoFromTemplate(r.Context(), cred, &req)
	}
	var notReady *repoNotReadyError
	switch {
	case errors.As(err, &notReady):
		writeErrResponse(w, err, notReady.status)
	case err != nil:
		s.writeGitProviderError(w, req.Provider, err)
	default:
		writeResponse(w, resp, http.StatusCreated)
	}
}

func normalizeCreateRepoFromTemplate(req *types.CreateRepoFromTemplateRequest) error {
	if req.Provider != types.ExternalRepositoryTypeGitHub && req.Provider != types.ExternalRepositoryTypeGitLab {
		return fmt.Errorf("provider must be github or gitlab, got %q", req.Provider)
	}
	if req.Owner == "" {
		return errors.New("owner is required")
	}
	if req.Name == "" {
		return errors.New("name is required")
	}
	if !templateRepoPattern.MatchString(req.Template) {
		return fmt.Errorf("template must be a github.com repository as owner/repo, got %q", req.Template)
	}
	if req.Private == nil {
		req.Private = gogithub.Bool(true)
	}
	return nil
}

// repoNotReadyError reports that a created repository did not become usable:
// the GitLab import failed (422) or readiness timed out (504).
type repoNotReadyError struct {
	status  int
	message string
}

func (e *repoNotReadyError) Error() string { return e.message }

func (s *HelixAPIServer) createGitHubRepoFromTemplate(ctx context.Context, cred *gitHostCredential, req *types.CreateRepoFromTemplateRequest) (*types.CreateRepoFromTemplateResponse, error) {
	templateOwner, templateRepo, _ := strings.Cut(req.Template, "/")
	client := s.gitHubTemplateClient(cred)
	repo, err := client.CreateRepositoryFromTemplate(ctx, templateOwner, templateRepo, req.Owner, req.Name, req.Description, *req.Private)
	if err != nil {
		return nil, err
	}
	owner := repo.GetOwner().GetLogin()
	branch := repo.GetDefaultBranch()
	if branch == "" {
		return nil, fmt.Errorf("GitHub returned no default branch for %s", repo.GetFullName())
	}
	// GitHub copies the template contents asynchronously; the repository is
	// clonable once its default branch exists.
	err = s.pollUntilReady(ctx, repo.GetFullName(), func(ctx context.Context) (bool, error) {
		return client.BranchExists(ctx, owner, repo.GetName(), branch)
	})
	if err != nil {
		return nil, err
	}
	return &types.CreateRepoFromTemplateResponse{
		CloneURL:      repo.GetCloneURL(),
		HTMLURL:       repo.GetHTMLURL(),
		DefaultBranch: branch,
		Provider:      types.ExternalRepositoryTypeGitHub,
		FullName:      repo.GetFullName(),
	}, nil
}

func (s *HelixAPIServer) createGitLabRepoFromTemplate(ctx context.Context, cred *gitHostCredential, req *types.CreateRepoFromTemplateRequest) (*types.CreateRepoFromTemplateResponse, error) {
	client, err := s.gitLabTemplateClient(cred)
	if err != nil {
		return nil, err
	}
	namespace, err := client.GetNamespace(ctx, req.Owner)
	if err != nil {
		return nil, err
	}
	visibility := gogitlab.PublicVisibility
	if *req.Private {
		visibility = gogitlab.PrivateVisibility
	}
	project, err := client.CreateProjectFromImportURL(ctx, req.Name, namespace.ID, visibility, req.Description, "https://github.com/"+req.Template+".git")
	if err != nil {
		return nil, err
	}
	err = s.pollUntilReady(ctx, project.PathWithNamespace, func(ctx context.Context) (bool, error) {
		status, err := client.GetImportStatus(ctx, project.ID)
		if err != nil {
			return false, err
		}
		switch status.ImportStatus {
		case "finished":
			return true, nil
		case "failed":
			return false, &repoNotReadyError{
				status: http.StatusUnprocessableEntity,
				message: fmt.Sprintf("gitlab: importing %s into %s failed: %s; the project was created, delete it before retrying",
					req.Template, project.PathWithNamespace, status.ImportError),
			}
		default:
			return false, nil
		}
	})
	if err != nil {
		return nil, err
	}
	project, err = client.GetProject(ctx, project.ID)
	if err != nil {
		return nil, err
	}
	return &types.CreateRepoFromTemplateResponse{
		CloneURL:      project.HTTPURLToRepo,
		HTMLURL:       project.WebURL,
		DefaultBranch: project.DefaultBranch,
		Provider:      types.ExternalRepositoryTypeGitLab,
		FullName:      project.PathWithNamespace,
	}, nil
}

// pollUntilReady calls ready every PollInterval until it reports true, fails,
// or ReadyTimeout passes.
func (s *HelixAPIServer) pollUntilReady(ctx context.Context, name string, ready func(context.Context) (bool, error)) error {
	ctx, cancel := context.WithTimeout(ctx, s.gitTemplate.ReadyTimeout)
	defer cancel()
	ticker := time.NewTicker(s.gitTemplate.PollInterval)
	defer ticker.Stop()
	for {
		ok, err := ready(ctx)
		if err != nil && ctx.Err() == nil {
			return err
		}
		if ok {
			return nil
		}
		if ctx.Err() != nil {
			return &repoNotReadyError{
				status:  http.StatusGatewayTimeout,
				message: fmt.Sprintf("%s was created but was not ready after %s", name, s.gitTemplate.ReadyTimeout),
			}
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
