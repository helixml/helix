package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	"github.com/rs/zerolog/log"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/agent/skill/bitbucket"
	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/agent/skill/gitlab"
	"github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/types"
)

// listGitProviderConnections returns all PAT-based git provider connections for the current user
// @Summary List git provider connections
// @Description List all PAT-based git provider connections for the current user
// @Tags git-provider-connections
// @Produce json
// @Success 200 {array} types.GitProviderConnection
// @Failure 401 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git-provider-connections [get]
// @Security BearerAuth
func (s *HelixAPIServer) listGitProviderConnections(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	connections, err := s.Store.ListGitProviderConnections(r.Context(), user.ID)
	if err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to list git provider connections")
		http.Error(w, fmt.Sprintf("Failed to list connections: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(connections)
}

// createGitProviderConnection creates a new PAT-based git provider connection
// @Summary Create git provider connection
// @Description Create a new PAT-based git provider connection for the current user
// @Tags git-provider-connections
// @Accept json
// @Produce json
// @Param request body types.GitProviderConnectionCreateRequest true "Connection details"
// @Success 201 {object} types.GitProviderConnection
// @Failure 400 {object} types.APIError
// @Failure 401 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git-provider-connections [post]
// @Security BearerAuth
func (s *HelixAPIServer) createGitProviderConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req types.GitProviderConnectionCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request: %s", err.Error()), http.StatusBadRequest)
		return
	}

	if req.Token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	if req.ProviderType == "" {
		http.Error(w, "Provider type is required", http.StatusBadRequest)
		return
	}

	if req.ProviderType == types.ExternalRepositoryTypeADO && req.OrganizationURL == "" {
		http.Error(w, "Organization URL is required for Azure DevOps", http.StatusBadRequest)
		return
	}

	if req.ProviderType == types.ExternalRepositoryTypeBitbucket && req.AuthUsername == "" {
		http.Error(w, "Username is required for Bitbucket", http.StatusBadRequest)
		return
	}

	// Validate the token by fetching user info
	userInfo, err := s.validateAndFetchUserInfo(r.Context(), req.ProviderType, req.Token, req.AuthUsername, req.OrganizationURL, req.BaseURL)
	if err != nil {
		// Upstream/transport error text (dial errors, dialed URLs, JSON parse
		// failures) must never reach the caller: it turns this endpoint into an
		// internal port-probing oracle (TST-01-271). Log it and return a
		// generic message unless the error was explicitly marked user-safe.
		log.Warn().Err(err).
			Str("user_id", user.ID).
			Str("provider_type", string(req.ProviderType)).
			Str("base_url", req.BaseURL).
			Str("organization_url", req.OrganizationURL).
			Msg("Git provider connection validation failed")
		http.Error(w, gitProviderClientErrorMessage(err, errInvalidProviderConnectionMsg), http.StatusBadRequest)
		return
	}

	// Generate default name if not provided
	name := req.Name
	if name == "" {
		name = fmt.Sprintf("%s (%s)", req.ProviderType, userInfo.Username)
	}

	// Encrypt the token before storage
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		http.Error(w, "Failed to encrypt token", http.StatusInternalServerError)
		return
	}

	encryptedToken, err := crypto.EncryptAES256GCM([]byte(req.Token), encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encrypt token")
		http.Error(w, "Failed to encrypt token", http.StatusInternalServerError)
		return
	}

	// Encrypt auth username for Bitbucket if provided
	var encryptedAuthUsername string
	if req.AuthUsername != "" {
		encryptedAuthUsername, err = crypto.EncryptAES256GCM([]byte(req.AuthUsername), encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to encrypt auth username")
			http.Error(w, "Failed to encrypt credentials", http.StatusInternalServerError)
			return
		}
	}

	now := time.Now()
	connection := &types.GitProviderConnection{
		ID:              uuid.New().String(),
		UserID:          user.ID,
		ProviderType:    req.ProviderType,
		Name:            name,
		Token:           encryptedToken,
		AuthUsername:    encryptedAuthUsername,
		OrganizationURL: req.OrganizationURL,
		BaseURL:         req.BaseURL,
		Username:        userInfo.Username,
		Email:           userInfo.Email,
		AvatarURL:       userInfo.AvatarURL,
		LastTestedAt:    &now,
	}

	if err := s.Store.CreateGitProviderConnection(r.Context(), connection); err != nil {
		log.Error().Err(err).Str("user_id", user.ID).Msg("Failed to create git provider connection")
		http.Error(w, fmt.Sprintf("Failed to create connection: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(connection)
}

// deleteGitProviderConnection deletes a PAT-based git provider connection
// @Summary Delete git provider connection
// @Description Delete a PAT-based git provider connection
// @Tags git-provider-connections
// @Param id path string true "Connection ID"
// @Success 204
// @Failure 401 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git-provider-connections/{id} [delete]
// @Security BearerAuth
func (s *HelixAPIServer) deleteGitProviderConnection(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	// Verify the connection belongs to this user
	connection, err := s.Store.GetGitProviderConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	if connection.UserID != user.ID {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	if err := s.Store.DeleteGitProviderConnection(r.Context(), connectionID); err != nil {
		log.Error().Err(err).Str("connection_id", connectionID).Msg("Failed to delete git provider connection")
		http.Error(w, fmt.Sprintf("Failed to delete connection: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// browseGitProviderConnectionRepositories lists repositories using a saved connection
// @Summary Browse repositories from saved connection
// @Description List repositories from a saved PAT-based git provider connection
// @Tags git-provider-connections
// @Produce json
// @Param id path string true "Connection ID"
// @Success 200 {object} types.ListOAuthRepositoriesResponse
// @Failure 401 {object} types.APIError
// @Failure 404 {object} types.APIError
// @Failure 500 {object} types.APIError
// @Router /api/v1/git-provider-connections/{id}/repositories [get]
// @Security BearerAuth
func (s *HelixAPIServer) browseGitProviderConnectionRepositories(w http.ResponseWriter, r *http.Request) {
	user := getRequestUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	connectionID := mux.Vars(r)["id"]
	if connectionID == "" {
		http.Error(w, "Connection ID is required", http.StatusBadRequest)
		return
	}

	// Get the connection and verify ownership
	connection, err := s.Store.GetGitProviderConnection(r.Context(), connectionID)
	if err != nil {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	if connection.UserID != user.ID {
		http.Error(w, "Connection not found", http.StatusNotFound)
		return
	}

	// Decrypt the token
	encryptionKey, err := s.getEncryptionKey()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get encryption key")
		http.Error(w, "Failed to decrypt token", http.StatusInternalServerError)
		return
	}

	decryptedToken, err := crypto.DecryptAES256GCM(connection.Token, encryptionKey)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decrypt token")
		http.Error(w, "Failed to decrypt token", http.StatusInternalServerError)
		return
	}

	// Decrypt auth username for Bitbucket if present
	var decryptedAuthUsername string
	if connection.AuthUsername != "" {
		decryptedAuthUsernameBytes, err := crypto.DecryptAES256GCM(connection.AuthUsername, encryptionKey)
		if err != nil {
			log.Error().Err(err).Msg("Failed to decrypt auth username")
			http.Error(w, "Failed to decrypt credentials", http.StatusInternalServerError)
			return
		}
		decryptedAuthUsername = string(decryptedAuthUsernameBytes)
	}

	// Use the existing browse logic
	req := types.BrowseRemoteRepositoriesRequest{
		ProviderType:    connection.ProviderType,
		Token:           string(decryptedToken),
		Username:        decryptedAuthUsername,
		OrganizationURL: connection.OrganizationURL,
		BaseURL:         connection.BaseURL,
	}

	repos, err := s.fetchRepositoriesWithPAT(r.Context(), req)
	if err != nil {
		log.Error().Err(err).Str("connection_id", connectionID).Msg("Failed to browse repositories")
		http.Error(w, gitProviderClientErrorMessage(err, "Failed to browse repositories. Check the connection's token and base URL, then try again."), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(types.ListOAuthRepositoriesResponse{
		Repositories: repos,
	})
}

// errInvalidProviderConnectionMsg is the single generic message returned when
// validating a git provider connection against the upstream API fails.
const errInvalidProviderConnectionMsg = "Invalid provider connection: check the token, username and base URL, then try again."

// gitProviderUserError is a validation failure whose message was written by us
// (missing fields, missing token scopes, ...) and is safe to show to the
// caller. Any other error from the provider path may carry transport details
// (dialed address, internal URL, response parse errors) and must only be logged.
type gitProviderUserError struct {
	msg string
}

func (e *gitProviderUserError) Error() string { return e.msg }

func newGitProviderUserError(format string, args ...any) error {
	return &gitProviderUserError{msg: fmt.Sprintf(format, args...)}
}

// gitProviderClientErrorMessage returns the message to send to the API caller
// for err: the user-safe message if err is (or wraps) a gitProviderUserError,
// otherwise fallback.
func gitProviderClientErrorMessage(err error, fallback string) string {
	var userErr *gitProviderUserError
	if errors.As(err, &userErr) {
		return userErr.msg
	}
	return fallback
}

// gitProviderConnectionHTTPClient checks a connection's self-hosted base URL
// and Azure DevOps organization URL against the git provider outbound policy
// and returns the client to use for baseURL (nil means the library default,
// for the public github.com / gitlab.com / bitbucket.org APIs). The Azure
// DevOps SDK builds its own client, so its URL is only checked up front.
func (s *HelixAPIServer) gitProviderConnectionHTTPClient(ctx context.Context, baseURL, orgURL string) (*http.Client, error) {
	policy := s.gitProviderURLPolicy()
	for field, raw := range map[string]string{"base URL": baseURL, "organization URL": orgURL} {
		if raw == "" {
			continue
		}
		if err := policy.ValidateURL(ctx, raw); err != nil {
			// Not a user error: callers answer with their generic message so
			// the response does not reveal how internal names resolve.
			return nil, fmt.Errorf("%s rejected by outbound policy: %w", field, err)
		}
	}
	return gitProviderHTTPClient(policy, baseURL), nil
}

// validateAndFetchUserInfo validates a PAT and fetches user info from the provider
func (s *HelixAPIServer) validateAndFetchUserInfo(ctx context.Context, providerType types.ExternalRepositoryType, token, authUsername, orgURL, baseURL string) (*types.OAuthUserInfo, error) {
	httpClient, err := s.gitProviderConnectionHTTPClient(ctx, baseURL, orgURL)
	if err != nil {
		return nil, err
	}
	switch providerType {
	case types.ExternalRepositoryTypeGitHub:
		client := github.NewClientWithPATBaseURLAndHTTPClient(token, baseURL, httpClient)
		user, scopes, scopeHeaderPresent, err := client.GetAuthenticatedUserWithScopes(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate GitHub token: %w", err)
		}
		if err := githubPATScopeError(scopes, scopeHeaderPresent); err != nil {
			return nil, err
		}
		return &types.OAuthUserInfo{
			ID:        fmt.Sprintf("%d", user.GetID()),
			Email:     user.GetEmail(),
			Name:      user.GetName(),
			Username:  user.GetLogin(),
			AvatarURL: user.GetAvatarURL(),
		}, nil

	case types.ExternalRepositoryTypeGitLab:
		client, err := gitlab.NewClientWithPATAndHTTPClient(baseURL, token, httpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitLab client: %w", err)
		}
		user, err := client.GetCurrentUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate GitLab token: %w", err)
		}
		return &types.OAuthUserInfo{
			ID:        fmt.Sprintf("%d", user.ID),
			Email:     user.Email,
			Name:      user.Name,
			Username:  user.Username,
			AvatarURL: user.AvatarURL,
		}, nil

	case types.ExternalRepositoryTypeADO:
		if orgURL == "" {
			return nil, newGitProviderUserError("organization URL is required for Azure DevOps")
		}
		client := azuredevops.NewAzureDevOpsClient(orgURL, token)
		profile, err := client.GetUserProfile(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate Azure DevOps token: %w", err)
		}
		return &types.OAuthUserInfo{
			ID:       profile.ID,
			Email:    profile.EmailAddress,
			Name:     profile.DisplayName,
			Username: profile.DisplayName, // ADO doesn't have a separate username
		}, nil

	case types.ExternalRepositoryTypeBitbucket:
		if authUsername == "" {
			return nil, newGitProviderUserError("username is required for Bitbucket")
		}
		client := bitbucket.NewClientWithHTTPClient(authUsername, token, baseURL, httpClient)
		user, err := client.GetCurrentUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate Bitbucket credentials: %w", err)
		}
		return &types.OAuthUserInfo{
			ID:       user.UUID,
			Email:    user.Email,
			Name:     user.DisplayName,
			Username: user.Username,
		}, nil

	default:
		return nil, newGitProviderUserError("unsupported provider type: %s", providerType)
	}
}

// fetchRepositoriesWithPAT fetches repositories using a PAT (used by both endpoints)
func (s *HelixAPIServer) fetchRepositoriesWithPAT(ctx context.Context, req types.BrowseRemoteRepositoriesRequest) ([]types.RepositoryInfo, error) {
	var repos []types.RepositoryInfo

	httpClient, err := s.gitProviderConnectionHTTPClient(ctx, req.BaseURL, req.OrganizationURL)
	if err != nil {
		return nil, err
	}

	switch req.ProviderType {
	case types.ExternalRepositoryTypeGitHub:
		ghClient := github.NewClientWithPATBaseURLAndHTTPClient(req.Token, req.BaseURL, httpClient)
		ghRepos, err := ghClient.ListRepositories(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list GitHub repositories: %w", err)
		}
		for _, repo := range ghRepos {
			repos = append(repos, types.RepositoryInfo{
				Name:          repo.GetName(),
				FullName:      repo.GetFullName(),
				CloneURL:      repo.GetCloneURL(),
				HTMLURL:       repo.GetHTMLURL(),
				Description:   repo.GetDescription(),
				Private:       repo.GetPrivate(),
				DefaultBranch: repo.GetDefaultBranch(),
				CanWrite:      github.HasWriteAccess(repo),
			})
		}

	case types.ExternalRepositoryTypeGitLab:
		glClient, err := gitlab.NewClientWithPATAndHTTPClient(req.BaseURL, req.Token, httpClient)
		if err != nil {
			return nil, fmt.Errorf("failed to create GitLab client: %w", err)
		}
		glProjects, err := glClient.ListProjects(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list GitLab projects: %w", err)
		}
		for _, project := range glProjects {
			repos = append(repos, types.RepositoryInfo{
				Name:          project.Name,
				FullName:      project.PathWithNamespace,
				CloneURL:      project.HTTPURLToRepo,
				HTMLURL:       project.WebURL,
				Description:   project.Description,
				Private:       project.Visibility != "public",
				DefaultBranch: project.DefaultBranch,
				CanWrite:      true, // GitLab permission introspection not yet wired up; preserve prior behaviour
			})
		}

	case types.ExternalRepositoryTypeADO:
		if req.OrganizationURL == "" {
			return nil, newGitProviderUserError("organization URL is required for Azure DevOps")
		}
		adoClient := azuredevops.NewAzureDevOpsClient(req.OrganizationURL, req.Token)
		adoRepos, err := adoClient.ListRepositories(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to list Azure DevOps repositories: %w", err)
		}
		for _, repo := range adoRepos {
			name := ""
			fullName := ""
			cloneURL := ""
			htmlURL := ""
			defaultBranch := ""
			if repo.Name != nil {
				name = *repo.Name
				fullName = *repo.Name
			}
			if repo.RemoteUrl != nil {
				cloneURL = *repo.RemoteUrl
			}
			if repo.WebUrl != nil {
				htmlURL = *repo.WebUrl
			}
			if repo.DefaultBranch != nil {
				defaultBranch = *repo.DefaultBranch
			}
			repos = append(repos, types.RepositoryInfo{
				Name:          name,
				FullName:      fullName,
				CloneURL:      cloneURL,
				HTMLURL:       htmlURL,
				Private:       true,
				DefaultBranch: defaultBranch,
				CanWrite:      true, // ADO permission introspection not yet wired up; preserve prior behaviour
			})
		}

	case types.ExternalRepositoryTypeBitbucket:
		if req.Username == "" {
			return nil, newGitProviderUserError("username is required for Bitbucket")
		}
		bbClient := bitbucket.NewClientWithHTTPClient(req.Username, req.Token, req.BaseURL, httpClient)
		bbRepos, err := bbClient.ListRepositories(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list Bitbucket repositories: %w", err)
		}
		for _, repo := range bbRepos {
			repos = append(repos, types.RepositoryInfo{
				Name:          repo.Name,
				FullName:      repo.FullName,
				CloneURL:      repo.CloneURL,
				HTMLURL:       repo.HTMLURL,
				Description:   repo.Description,
				Private:       repo.IsPrivate,
				DefaultBranch: repo.MainBranch,
				CanWrite:      true, // Bitbucket permission introspection not yet wired up; preserve prior behaviour
			})
		}

	default:
		return nil, newGitProviderUserError("unsupported provider type: %s", req.ProviderType)
	}

	return repos, nil
}

// githubPATRequiredScopes are the classic-token scopes Helix needs: "repo" to
// clone and push, "workflow" to push changes under .github/workflows.
var githubPATRequiredScopes = []string{"repo", "workflow"}

// githubPATScopeError reports why a GitHub PAT can't be used, or nil if it has
// every required scope. Browsing repos with a token and saving it as a
// connection both check it, so a token is rejected on entry rather than when
// it is saved.
func githubPATScopeError(scopes []string, scopeHeaderPresent bool) error {
	if !scopeHeaderPresent {
		// Fine-grained PATs don't return X-OAuth-Scopes so we can't validate
		// their permissions. Reject them and ask for a classic token.
		return newGitProviderUserError("Fine-grained GitHub tokens are not supported. Helix requires a classic token with the 'repo' and 'workflow' scopes. Please create one at https://github.com/settings/tokens")
	}
	if len(scopes) == 0 {
		return newGitProviderUserError("GitHub token has no scopes. Helix requires the 'repo' and 'workflow' scopes. Please create a new classic token with them at https://github.com/settings/tokens")
	}
	var missing []string
	for _, required := range githubPATRequiredScopes {
		if !slices.Contains(scopes, required) {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		return newGitProviderUserError("GitHub token is missing required scopes: %s. Your token has scopes: %s. Please create a new classic token with the 'repo' and 'workflow' scopes at https://github.com/settings/tokens", strings.Join(missing, ", "), strings.Join(scopes, ", "))
	}
	return nil
}
