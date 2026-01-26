package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// checkSampleProjectAccess godoc
// @Summary Check repository access for a sample project
// @Description Check if the authenticated user has write access to the required repositories for a sample project
// @Tags    sample-projects
// @Accept  json
// @Produce json
// @Param   request body types.CheckSampleProjectAccessRequest true "Access check request"
// @Success 200 {object} types.CheckSampleProjectAccessResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/sample-projects/simple/check-access [post]
// @Security BearerAuth
func (s *HelixAPIServer) checkSampleProjectAccess(_ http.ResponseWriter, r *http.Request) (*types.CheckSampleProjectAccessResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req types.CheckSampleProjectAccessRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	// Find the sample project
	var sampleProject *SimpleSampleProject
	for _, project := range SIMPLE_SAMPLE_PROJECTS {
		if project.ID == req.SampleProjectID {
			sampleProject = &project
			break
		}
	}

	if sampleProject == nil {
		return nil, system.NewHTTPError404("sample project not found")
	}

	// Check if this sample project requires GitHub auth
	if !sampleProject.RequiresGitHubAuth || len(sampleProject.RequiredGitHubRepos) == 0 {
		// No GitHub auth required, all repos accessible anonymously
		return &types.CheckSampleProjectAccessResponse{
			SampleProjectID:    req.SampleProjectID,
			HasGitHubConnected: false,
			Repositories:       []types.RepositoryAccessCheck{},
			AllHaveWriteAccess: true,
		}, nil
	}

	// Get GitHub OAuth connection
	var accessToken string
	var gitHubUsername string

	if req.GitHubConnectionID != "" {
		// Use specified connection
		connection, err := s.Store.GetOAuthConnection(ctx, req.GitHubConnectionID)
		if err != nil {
			return nil, system.NewHTTPError404("GitHub connection not found")
		}
		if connection.UserID != user.ID {
			return nil, system.NewHTTPError403("not authorized to use this connection")
		}
		accessToken = connection.AccessToken
		gitHubUsername = connection.ProviderUsername
	} else {
		// Try to find user's GitHub OAuth connection
		connections, err := s.Store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
			UserID: user.ID,
		})
		if err != nil {
			log.Warn().Err(err).Str("user_id", user.ID).Msg("Failed to list OAuth connections")
		}

		for _, conn := range connections {
			if conn.Provider.Type == types.OAuthProviderTypeGitHub {
				accessToken = conn.AccessToken
				gitHubUsername = conn.ProviderUsername
				break
			}
		}
	}

	if accessToken == "" {
		// No GitHub connection found
		return &types.CheckSampleProjectAccessResponse{
			SampleProjectID:    req.SampleProjectID,
			HasGitHubConnected: false,
			Repositories:       buildRepoAccessChecksWithoutAuth(sampleProject.RequiredGitHubRepos),
			AllHaveWriteAccess: false,
		}, nil
	}

	// Create GitHub client with OAuth token
	ghClient := github.NewClientWithOAuth(accessToken)

	// Get authenticated user info if we don't have it
	if gitHubUsername == "" {
		ghUser, err := ghClient.GetAuthenticatedUser(ctx)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to get GitHub user info")
		} else {
			gitHubUsername = ghUser.GetLogin()
		}
	}

	// Check access to each required repository
	repoChecks := make([]types.RepositoryAccessCheck, 0, len(sampleProject.RequiredGitHubRepos))
	allHaveWriteAccess := true

	for _, reqRepo := range sampleProject.RequiredGitHubRepos {
		owner, repo, err := parseGitHubURLSimple(reqRepo.GitHubURL)
		if err != nil {
			log.Warn().Err(err).Str("url", reqRepo.GitHubURL).Msg("Failed to parse GitHub URL")
			continue
		}

		check := types.RepositoryAccessCheck{
			GitHubURL:     reqRepo.GitHubURL,
			Owner:         owner,
			Repo:          repo,
			IsPrimary:     reqRepo.IsPrimary,
			DefaultBranch: reqRepo.DefaultBranch,
			CanFork:       reqRepo.AllowFork,
		}

		// Check write access
		hasPush, err := ghClient.HasPushAccess(ctx, owner, repo)
		if err != nil {
			log.Warn().Err(err).Str("repo", reqRepo.GitHubURL).Msg("Failed to check push access")
			check.HasWriteAccess = false
		} else {
			check.HasWriteAccess = hasPush
		}

		// Check for existing fork if user doesn't have write access
		if !check.HasWriteAccess && reqRepo.AllowFork && gitHubUsername != "" {
			existingFork, err := ghClient.ForkExists(ctx, owner, repo, gitHubUsername)
			if err != nil {
				log.Warn().Err(err).Str("repo", reqRepo.GitHubURL).Msg("Failed to check for existing fork")
			} else if existingFork != nil {
				check.ExistingFork = existingFork.GetHTMLURL()
			}
		}

		if !check.HasWriteAccess {
			allHaveWriteAccess = false
		}

		repoChecks = append(repoChecks, check)
	}

	return &types.CheckSampleProjectAccessResponse{
		SampleProjectID:    req.SampleProjectID,
		HasGitHubConnected: true,
		GitHubUsername:     gitHubUsername,
		Repositories:       repoChecks,
		AllHaveWriteAccess: allHaveWriteAccess,
	}, nil
}

// forkSampleProjectRepositories godoc
// @Summary Fork repositories for a sample project
// @Description Fork the specified repositories to the user's GitHub account
// @Tags    sample-projects
// @Accept  json
// @Produce json
// @Param   request body types.ForkRepositoriesRequest true "Fork request"
// @Success 200 {object} types.ForkRepositoriesResponse
// @Failure 400 {object} system.HTTPError
// @Failure 401 {object} system.HTTPError
// @Failure 404 {object} system.HTTPError
// @Router /api/v1/sample-projects/simple/fork-repos [post]
// @Security BearerAuth
func (s *HelixAPIServer) forkSampleProjectRepositories(_ http.ResponseWriter, r *http.Request) (*types.ForkRepositoriesResponse, *system.HTTPError) {
	ctx := r.Context()
	user := getRequestUser(r)
	if user == nil {
		return nil, system.NewHTTPError401("unauthorized")
	}

	var req types.ForkRepositoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, system.NewHTTPError400("invalid request body")
	}

	if len(req.RepositoriesToFork) == 0 {
		return nil, system.NewHTTPError400("no repositories to fork")
	}

	// Get GitHub OAuth connection
	accessToken, err := s.getGitHubAccessToken(ctx, user.ID, req.GitHubConnectionID)
	if err != nil {
		return nil, system.NewHTTPError400("GitHub connection required: " + err.Error())
	}

	// Create GitHub client
	ghClient := github.NewClientWithOAuth(accessToken)

	// Fork each repository
	forkedRepos := make([]types.ForkedRepository, 0, len(req.RepositoriesToFork))

	for _, repoURL := range req.RepositoriesToFork {
		owner, repo, err := parseGitHubURLSimple(repoURL)
		if err != nil {
			log.Warn().Err(err).Str("url", repoURL).Msg("Failed to parse GitHub URL for fork")
			continue
		}

		log.Info().
			Str("user_id", user.ID).
			Str("owner", owner).
			Str("repo", repo).
			Str("fork_to_org", req.ForkToOrganization).
			Msg("Forking repository")

		forkedRepo, err := ghClient.ForkRepository(ctx, owner, repo, req.ForkToOrganization)
		if err != nil {
			log.Error().Err(err).Str("repo", repoURL).Msg("Failed to fork repository")
			continue
		}

		forkedRepos = append(forkedRepos, types.ForkedRepository{
			OriginalURL: repoURL,
			ForkedURL:   fmt.Sprintf("github.com/%s/%s", forkedRepo.GetOwner().GetLogin(), forkedRepo.GetName()),
			Owner:       forkedRepo.GetOwner().GetLogin(),
			Repo:        forkedRepo.GetName(),
		})

		log.Info().
			Str("user_id", user.ID).
			Str("forked_url", forkedRepo.GetHTMLURL()).
			Msg("Successfully forked repository")
	}

	return &types.ForkRepositoriesResponse{
		ForkedRepositories: forkedRepos,
	}, nil
}

// getGitHubAccessToken retrieves a GitHub OAuth access token for the user
func (s *HelixAPIServer) getGitHubAccessToken(ctx context.Context, userID, connectionID string) (string, error) {
	if connectionID != "" {
		// Use specified connection
		connection, err := s.Store.GetOAuthConnection(ctx, connectionID)
		if err != nil {
			return "", fmt.Errorf("connection not found")
		}
		if connection.UserID != userID {
			return "", fmt.Errorf("not authorized to use this connection")
		}
		return connection.AccessToken, nil
	}

	// Try to find user's GitHub OAuth connection
	connections, err := s.Store.ListOAuthConnections(ctx, &store.ListOAuthConnectionsQuery{
		UserID: userID,
	})
	if err != nil {
		return "", fmt.Errorf("failed to list connections: %w", err)
	}

	for _, conn := range connections {
		if conn.Provider.Type == types.OAuthProviderTypeGitHub {
			return conn.AccessToken, nil
		}
	}

	return "", fmt.Errorf("no GitHub connection found")
}

// parseGitHubURLSimple parses a GitHub URL and returns owner and repo
// Supports: github.com/owner/repo, https://github.com/owner/repo, etc.
func parseGitHubURLSimple(url string) (owner, repo string, err error) {
	// Remove common prefixes
	url = strings.TrimPrefix(url, "https://")
	url = strings.TrimPrefix(url, "http://")
	url = strings.TrimPrefix(url, "github.com/")
	url = strings.TrimSuffix(url, ".git")
	url = strings.TrimSuffix(url, "/")

	// Split by /
	parts := strings.Split(url, "/")
	if len(parts) >= 2 {
		return parts[0], parts[1], nil
	}

	return "", "", fmt.Errorf("invalid GitHub URL: %s", url)
}

// buildRepoAccessChecksWithoutAuth builds access checks when no GitHub auth is available
func buildRepoAccessChecksWithoutAuth(repos []RequiredGitHubRepo) []types.RepositoryAccessCheck {
	checks := make([]types.RepositoryAccessCheck, 0, len(repos))
	for _, reqRepo := range repos {
		owner, repo, _ := parseGitHubURLSimple(reqRepo.GitHubURL)
		checks = append(checks, types.RepositoryAccessCheck{
			GitHubURL:      reqRepo.GitHubURL,
			Owner:          owner,
			Repo:           repo,
			HasWriteAccess: false,
			CanFork:        reqRepo.AllowFork,
			IsPrimary:      reqRepo.IsPrimary,
			DefaultBranch:  reqRepo.DefaultBranch,
		})
	}
	return checks
}
