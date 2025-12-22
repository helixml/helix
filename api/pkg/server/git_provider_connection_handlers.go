package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
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

	json.NewEncoder(w).Encode( connections)
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
		http.Error(w, fmt.Sprintf("Invalid token or failed to connect: %s", err.Error()), http.StatusBadRequest)
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
	json.NewEncoder(w).Encode( connection)
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
		http.Error(w, fmt.Sprintf("Failed to browse repositories: %s", err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode( types.ListOAuthRepositoriesResponse{
		Repositories: repos,
	})
}

// validateAndFetchUserInfo validates a PAT and fetches user info from the provider
func (s *HelixAPIServer) validateAndFetchUserInfo(ctx context.Context, providerType types.ExternalRepositoryType, token, authUsername, orgURL, baseURL string) (*types.OAuthUserInfo, error) {
	switch providerType {
	case types.ExternalRepositoryTypeGitHub:
		client := github.NewClientWithPATAndBaseURL(token, baseURL)
		user, err := client.GetAuthenticatedUser(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to validate GitHub token: %w", err)
		}
		return &types.OAuthUserInfo{
			ID:        fmt.Sprintf("%d", user.GetID()),
			Email:     user.GetEmail(),
			Name:      user.GetName(),
			Username:  user.GetLogin(),
			AvatarURL: user.GetAvatarURL(),
		}, nil

	case types.ExternalRepositoryTypeGitLab:
		client, err := gitlab.NewClientWithPAT(baseURL, token)
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
			return nil, fmt.Errorf("organization URL is required for Azure DevOps")
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
			return nil, fmt.Errorf("username is required for Bitbucket")
		}
		client := bitbucket.NewClient(authUsername, token, baseURL)
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
		return nil, fmt.Errorf("unsupported provider type: %s", providerType)
	}
}

// fetchRepositoriesWithPAT fetches repositories using a PAT (used by both endpoints)
func (s *HelixAPIServer) fetchRepositoriesWithPAT(ctx context.Context, req types.BrowseRemoteRepositoriesRequest) ([]types.RepositoryInfo, error) {
	var repos []types.RepositoryInfo

	switch req.ProviderType {
	case types.ExternalRepositoryTypeGitHub:
		ghClient := github.NewClientWithPATAndBaseURL(req.Token, req.BaseURL)
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
			})
		}

	case types.ExternalRepositoryTypeGitLab:
		glClient, err := gitlab.NewClientWithPAT(req.BaseURL, req.Token)
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
			})
		}

	case types.ExternalRepositoryTypeADO:
		if req.OrganizationURL == "" {
			return nil, fmt.Errorf("organization URL is required for Azure DevOps")
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
			})
		}

	case types.ExternalRepositoryTypeBitbucket:
		if req.Username == "" {
			return nil, fmt.Errorf("username is required for Bitbucket")
		}
		bbClient := bitbucket.NewClient(req.Username, req.Token, req.BaseURL)
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
			})
		}

	default:
		return nil, fmt.Errorf("unsupported provider type: %s", req.ProviderType)
	}

	return repos, nil
}
