package services

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	azuredevops "github.com/helixml/helix/api/pkg/agent/skill/azure_devops"
	"github.com/helixml/helix/api/pkg/agent/skill/bitbucket"
	"github.com/helixml/helix/api/pkg/agent/skill/github"
	"github.com/helixml/helix/api/pkg/agent/skill/gitlab"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// CreatePullRequest opens a pull request in the external repository. Should be called after the changes are committed to the local repository and
// it has been pushed to the external repository.
func (s *GitRepositoryService) CreatePullRequest(ctx context.Context, repoID string, title string, description string, sourceBranch string, targetBranch string) (string, error) {
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return "", fmt.Errorf("repository not found: %w", err)
	}

	if repo.ExternalURL == "" {
		return "", fmt.Errorf("repository is not external, cannot create pull request")
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeADO:
		return s.createAzureDevOpsPullRequest(ctx, repo, title, description, sourceBranch, targetBranch)
	case types.ExternalRepositoryTypeGitHub:
		return s.createGitHubPullRequest(ctx, repo, title, description, sourceBranch, targetBranch)
	case types.ExternalRepositoryTypeGitLab:
		return s.createGitLabMergeRequest(ctx, repo, title, description, sourceBranch, targetBranch)
	case types.ExternalRepositoryTypeBitbucket:
		return s.createBitbucketPullRequest(ctx, repo, title, description, sourceBranch, targetBranch)
	default:
		return "", fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (s *GitRepositoryService) createAzureDevOpsPullRequest(ctx context.Context, repo *types.GitRepository, title string, description string, sourceBranch string, targetBranch string) (string, error) {

	if repo.AzureDevOps == nil {
		return "", fmt.Errorf("azure devops repository not found")
	}

	if repo.AzureDevOps.OrganizationURL == "" {
		return "", fmt.Errorf("azure devops organization URL not found")
	}

	if repo.AzureDevOps.PersonalAccessToken == "" {
		return "", fmt.Errorf("azure devops personal access token not found, get yours from https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows")
	}

	client := azuredevops.NewAzureDevOpsClient(repo.AzureDevOps.OrganizationURL, repo.AzureDevOps.PersonalAccessToken)

	project, err := s.getAzureDevOpsProject(repo)
	if err != nil {
		return "", fmt.Errorf("failed to get azure devops project: %w", err)
	}

	repositoryName, err := s.getAzureDevOpsRepositoryName(repo)
	if err != nil {
		return "", fmt.Errorf("failed to get azure devops repository name: %w", err)
	}

	pr, err := client.CreatePullRequest(ctx, repositoryName, title, description, sourceBranch, targetBranch, project)
	if err != nil {
		log.Error().Err(err).Str("repository_name", repositoryName).
			Str("project", project).
			Str("title", title).
			Str("description", description).
			Str("source_branch", sourceBranch).
			Str("target_branch", targetBranch).
			Msg("failed to create pull request")
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	if pr.PullRequestId == nil {
		return "", fmt.Errorf("pull request ID is nil")
	}

	id := strconv.Itoa(*pr.PullRequestId)

	return id, nil
}

func (s *GitRepositoryService) ListPullRequests(ctx context.Context, repoID string) ([]*types.PullRequest, error) {
	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.ExternalURL == "" {
		return nil, fmt.Errorf("repository is not external, cannot list pull requests")
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeADO:
		return s.listAzureDevOpsPullRequests(ctx, repo)
	case types.ExternalRepositoryTypeGitHub:
		return s.listGitHubPullRequests(ctx, repo)
	case types.ExternalRepositoryTypeGitLab:
		return s.listGitLabMergeRequests(ctx, repo)
	case types.ExternalRepositoryTypeBitbucket:
		return s.listBitbucketPullRequests(ctx, repo)
	default:
		return nil, fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (s *GitRepositoryService) listAzureDevOpsPullRequests(ctx context.Context, repo *types.GitRepository) ([]*types.PullRequest, error) {
	if repo.AzureDevOps == nil {
		return nil, fmt.Errorf("azure devops repository not found")
	}

	if repo.AzureDevOps.OrganizationURL == "" {
		return nil, fmt.Errorf("azure devops organization URL not found")
	}

	if repo.AzureDevOps.PersonalAccessToken == "" {
		return nil, fmt.Errorf("azure devops personal access token not found, get yours from https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows")
	}

	client := azuredevops.NewAzureDevOpsClient(repo.AzureDevOps.OrganizationURL, repo.AzureDevOps.PersonalAccessToken)

	// Get azure project ID
	project, err := s.getAzureDevOpsProject(repo)
	if err != nil {
		return nil, err
	}

	repositoryName, err := s.getAzureDevOpsRepositoryName(repo)
	if err != nil {
		return nil, err
	}

	gitPRs, err := client.ListPullRequests(ctx, repositoryName, project)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(gitPRs))
	for _, gitPR := range gitPRs {
		pr := &types.PullRequest{}

		if gitPR.PullRequestId != nil {
			pr.Number = *gitPR.PullRequestId
			pr.ID = strconv.Itoa(*gitPR.PullRequestId)
		}

		if gitPR.Title != nil {
			pr.Title = *gitPR.Title
		}

		if gitPR.Description != nil {
			pr.Description = *gitPR.Description
		}

		if gitPR.Status != nil {
			pr.State = string(*gitPR.Status)
		}

		if gitPR.SourceRefName != nil {
			pr.SourceBranch = *gitPR.SourceRefName
		}

		if gitPR.TargetRefName != nil {
			pr.TargetBranch = *gitPR.TargetRefName
		}

		if gitPR.CreationDate != nil {
			pr.CreatedAt = gitPR.CreationDate.Time
		}

		if gitPR.CreationDate != nil {
			pr.UpdatedAt = gitPR.CreationDate.Time
		}

		if gitPR.CreatedBy != nil && gitPR.CreatedBy.DisplayName != nil {
			pr.Author = *gitPR.CreatedBy.DisplayName
		}

		if gitPR.Url != nil {
			pr.URL = fmt.Sprintf("%s/pullrequest/%d", repo.ExternalURL, *gitPR.PullRequestId)
		}

		prs = append(prs, pr)
	}

	return prs, nil
}

func (s *GitRepositoryService) GetPullRequest(ctx context.Context, repoID, id string) (*types.PullRequest, error) {

	repo, err := s.GetRepository(ctx, repoID)
	if err != nil {
		return nil, fmt.Errorf("repository not found: %w", err)
	}

	if repo.ExternalURL == "" {
		return nil, fmt.Errorf("repository is not external, cannot get pull request")
	}

	switch repo.ExternalType {
	case types.ExternalRepositoryTypeADO:
		prID, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("invalid pull request ID: %w (Azure DevOps ID is an integer)", err)
		}
		return s.getAzureDevOpsPullRequest(ctx, repo, prID)
	case types.ExternalRepositoryTypeGitHub:
		prNumber, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("invalid pull request number: %w", err)
		}
		return s.getGitHubPullRequest(ctx, repo, prNumber)
	case types.ExternalRepositoryTypeGitLab:
		mrIID, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("invalid merge request IID: %w", err)
		}
		return s.getGitLabMergeRequest(ctx, repo, mrIID)
	case types.ExternalRepositoryTypeBitbucket:
		prID, err := strconv.Atoi(id)
		if err != nil {
			return nil, fmt.Errorf("invalid pull request ID: %w", err)
		}
		return s.getBitbucketPullRequest(ctx, repo, prID)
	default:
		return nil, fmt.Errorf("unsupported external repository type: %s", repo.ExternalType)
	}
}

func (s *GitRepositoryService) getAzureDevOpsPullRequest(ctx context.Context, repo *types.GitRepository, id int) (*types.PullRequest, error) {
	if repo.AzureDevOps == nil {
		return nil, fmt.Errorf("azure devops repository not found")
	}

	if repo.AzureDevOps.OrganizationURL == "" {
		return nil, fmt.Errorf("azure devops organization URL not found")
	}

	client := azuredevops.NewAzureDevOpsClient(repo.AzureDevOps.OrganizationURL, repo.AzureDevOps.PersonalAccessToken)

	project, err := s.getAzureDevOpsProject(repo)
	if err != nil {
		return nil, err
	}

	repositoryName, err := s.getAzureDevOpsRepositoryName(repo)
	if err != nil {
		return nil, err
	}

	adoPR, err := client.GetPullRequest(ctx, repositoryName, project, id)
	if err != nil {
		return nil, err
	}

	pr := &types.PullRequest{}

	if adoPR.PullRequestId != nil {
		pr.Number = *adoPR.PullRequestId
		pr.ID = strconv.Itoa(*adoPR.PullRequestId)
	}

	if adoPR.Title != nil {
		pr.Title = *adoPR.Title
	}

	if adoPR.Description != nil {
		pr.Description = *adoPR.Description
	}

	if adoPR.Status != nil {
		pr.State = string(*adoPR.Status)
	}

	if adoPR.SourceRefName != nil {
		pr.SourceBranch = *adoPR.SourceRefName
	}

	if adoPR.TargetRefName != nil {
		pr.TargetBranch = *adoPR.TargetRefName
	}

	if adoPR.CreationDate != nil {
		pr.CreatedAt = adoPR.CreationDate.Time
	}

	if adoPR.CreationDate != nil {
		pr.UpdatedAt = adoPR.CreationDate.Time
	}

	if adoPR.CreatedBy != nil && adoPR.CreatedBy.DisplayName != nil {
		pr.Author = *adoPR.CreatedBy.DisplayName
	}

	if adoPR.Url != nil {
		pr.URL = fmt.Sprintf("%s/pullrequest/%d", repo.ExternalURL, *adoPR.PullRequestId)
	}

	return pr, nil
}

func (s *GitRepositoryService) getAzureDevOpsProject(repo *types.GitRepository) (string, error) {
	// parse project from ExternalURL
	// expected format: https://dev.azure.com/{org}/{project}/_git/{repo}
	// or https://{org}.visualstudio.com/{project}/_git/{repo}

	u, err := url.Parse(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("invalid external URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Find "_git" and take the part before it
	for i, part := range pathParts {
		if part == "_git" {
			if i > 0 {
				return pathParts[i-1], nil
			}
			break
		}
	}

	return "", fmt.Errorf("could not parse project from URL: %s", repo.ExternalURL)
}

func (s *GitRepositoryService) getAzureDevOpsRepositoryName(repo *types.GitRepository) (string, error) {
	// parse repository name from ExternalURL
	// expected format: https://dev.azure.com/{org}/{project}/_git/{repo}
	// or https://{org}.visualstudio.com/{project}/_git/{repo}

	u, err := url.Parse(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("invalid external URL: %w", err)
	}

	pathParts := strings.Split(strings.Trim(u.Path, "/"), "/")

	// Find "_git" and take the part after it
	for i, part := range pathParts {
		if part == "_git" {
			if i < len(pathParts)-1 {
				return pathParts[i+1], nil
			}
		}
	}

	return "", fmt.Errorf("could not parse repository name from URL: %s", repo.ExternalURL)
}

// GitHub Pull Request Operations

func (s *GitRepositoryService) getGitHubClient(ctx context.Context, repo *types.GitRepository) (*github.Client, error) {
	// Get GitHub Enterprise base URL if configured
	var baseURL string
	if repo.GitHub != nil {
		baseURL = repo.GitHub.BaseURL
	}

	// First check for OAuth connection
	if repo.OAuthConnectionID != "" {
		conn, err := s.store.GetOAuthConnection(ctx, repo.OAuthConnectionID)
		if err == nil && conn.AccessToken != "" {
			return github.NewClientWithOAuthAndBaseURL(conn.AccessToken, baseURL), nil
		}
	}

	// Check for GitHub-specific PAT
	if repo.GitHub != nil && repo.GitHub.PersonalAccessToken != "" {
		return github.NewClientWithPATAndBaseURL(repo.GitHub.PersonalAccessToken, baseURL), nil
	}

	// Fall back to username/password (password is typically a PAT)
	if repo.Password != "" {
		return github.NewClientWithPATAndBaseURL(repo.Password, baseURL), nil
	}

	return nil, fmt.Errorf("no GitHub authentication configured - provide a Personal Access Token or connect via OAuth")
}

func (s *GitRepositoryService) createGitHubPullRequest(ctx context.Context, repo *types.GitRepository, title string, description string, sourceBranch string, targetBranch string) (string, error) {
	client, err := s.getGitHubClient(ctx, repo)
	if err != nil {
		return "", err
	}

	owner, repoName, err := github.ParseGitHubURL(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	pr, err := client.CreatePullRequest(ctx, owner, repoName, title, description, sourceBranch, targetBranch)
	if err != nil {
		log.Error().Err(err).
			Str("owner", owner).
			Str("repo", repoName).
			Str("title", title).
			Str("source_branch", sourceBranch).
			Str("target_branch", targetBranch).
			Msg("failed to create GitHub pull request")
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	return strconv.Itoa(pr.GetNumber()), nil
}

func (s *GitRepositoryService) listGitHubPullRequests(ctx context.Context, repo *types.GitRepository) ([]*types.PullRequest, error) {
	client, err := s.getGitHubClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := github.ParseGitHubURL(repo.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	ghPRs, err := client.ListPullRequests(ctx, owner, repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(ghPRs))
	for _, ghPR := range ghPRs {
		pr := &types.PullRequest{
			ID:           strconv.Itoa(ghPR.GetNumber()),
			Number:       ghPR.GetNumber(),
			Title:        ghPR.GetTitle(),
			Description:  ghPR.GetBody(),
			State:        ghPR.GetState(),
			SourceBranch: ghPR.GetHead().GetRef(),
			TargetBranch: ghPR.GetBase().GetRef(),
			URL:          ghPR.GetHTMLURL(),
		}

		if ghPR.GetUser() != nil {
			pr.Author = ghPR.GetUser().GetLogin()
		}

		if ghPR.CreatedAt != nil {
			pr.CreatedAt = ghPR.CreatedAt.Time
		}

		if ghPR.UpdatedAt != nil {
			pr.UpdatedAt = ghPR.UpdatedAt.Time
		}

		prs = append(prs, pr)
	}

	return prs, nil
}

func (s *GitRepositoryService) getGitHubPullRequest(ctx context.Context, repo *types.GitRepository, number int) (*types.PullRequest, error) {
	client, err := s.getGitHubClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	owner, repoName, err := github.ParseGitHubURL(repo.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse GitHub URL: %w", err)
	}

	ghPR, err := client.GetPullRequest(ctx, owner, repoName, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	pr := &types.PullRequest{
		ID:           strconv.Itoa(ghPR.GetNumber()),
		Number:       ghPR.GetNumber(),
		Title:        ghPR.GetTitle(),
		Description:  ghPR.GetBody(),
		State:        ghPR.GetState(),
		SourceBranch: ghPR.GetHead().GetRef(),
		TargetBranch: ghPR.GetBase().GetRef(),
		URL:          ghPR.GetHTMLURL(),
	}

	if ghPR.GetUser() != nil {
		pr.Author = ghPR.GetUser().GetLogin()
	}

	if ghPR.CreatedAt != nil {
		pr.CreatedAt = ghPR.CreatedAt.Time
	}

	if ghPR.UpdatedAt != nil {
		pr.UpdatedAt = ghPR.UpdatedAt.Time
	}

	return pr, nil
}

// GitLab Merge Request Operations

func (s *GitRepositoryService) getGitLabClient(ctx context.Context, repo *types.GitRepository) (*gitlab.Client, error) {
	// Determine base URL (empty for gitlab.com, custom for self-hosted)
	var baseURL string
	if repo.GitLab != nil && repo.GitLab.BaseURL != "" {
		baseURL = repo.GitLab.BaseURL
	} else {
		// Try to extract from ExternalURL if it's not gitlab.com
		parsedBaseURL, _, err := gitlab.ParseGitLabURL(repo.ExternalURL)
		if err == nil && parsedBaseURL != "" {
			baseURL = parsedBaseURL
		}
	}

	// First check for OAuth connection
	if repo.OAuthConnectionID != "" {
		conn, err := s.store.GetOAuthConnection(ctx, repo.OAuthConnectionID)
		if err == nil && conn.AccessToken != "" {
			return gitlab.NewClientWithOAuth(baseURL, conn.AccessToken)
		}
	}

	// Check for GitLab-specific PAT
	if repo.GitLab != nil && repo.GitLab.PersonalAccessToken != "" {
		return gitlab.NewClientWithPAT(baseURL, repo.GitLab.PersonalAccessToken)
	}

	// Fall back to username/password (password is typically a PAT)
	if repo.Password != "" {
		return gitlab.NewClientWithPAT(baseURL, repo.Password)
	}

	return nil, fmt.Errorf("no GitLab authentication configured - provide a Personal Access Token or connect via OAuth")
}

func (s *GitRepositoryService) getGitLabProjectID(ctx context.Context, client *gitlab.Client, repo *types.GitRepository) (int, error) {
	_, projectPath, err := gitlab.ParseGitLabURL(repo.ExternalURL)
	if err != nil {
		return 0, fmt.Errorf("failed to parse GitLab URL: %w", err)
	}

	project, err := client.GetProjectByPath(ctx, projectPath)
	if err != nil {
		return 0, fmt.Errorf("failed to get project: %w", err)
	}

	return project.ID, nil
}

func (s *GitRepositoryService) createGitLabMergeRequest(ctx context.Context, repo *types.GitRepository, title string, description string, sourceBranch string, targetBranch string) (string, error) {
	client, err := s.getGitLabClient(ctx, repo)
	if err != nil {
		return "", err
	}

	projectID, err := s.getGitLabProjectID(ctx, client, repo)
	if err != nil {
		return "", err
	}

	mr, err := client.CreateMergeRequest(ctx, projectID, title, description, sourceBranch, targetBranch)
	if err != nil {
		log.Error().Err(err).
			Int("project_id", projectID).
			Str("title", title).
			Str("source_branch", sourceBranch).
			Str("target_branch", targetBranch).
			Msg("failed to create GitLab merge request")
		return "", fmt.Errorf("failed to create merge request: %w", err)
	}

	return strconv.Itoa(mr.IID), nil
}

func (s *GitRepositoryService) listGitLabMergeRequests(ctx context.Context, repo *types.GitRepository) ([]*types.PullRequest, error) {
	client, err := s.getGitLabClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	projectID, err := s.getGitLabProjectID(ctx, client, repo)
	if err != nil {
		return nil, err
	}

	glMRs, err := client.ListMergeRequests(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(glMRs))
	for _, glMR := range glMRs {
		pr := &types.PullRequest{
			ID:           strconv.Itoa(glMR.IID),
			Number:       glMR.IID,
			Title:        glMR.Title,
			Description:  glMR.Description,
			State:        glMR.State,
			SourceBranch: glMR.SourceBranch,
			TargetBranch: glMR.TargetBranch,
			URL:          glMR.WebURL,
		}

		if glMR.Author != nil {
			pr.Author = glMR.Author.Username
		}

		if glMR.CreatedAt != nil {
			pr.CreatedAt = *glMR.CreatedAt
		}

		if glMR.UpdatedAt != nil {
			pr.UpdatedAt = *glMR.UpdatedAt
		}

		prs = append(prs, pr)
	}

	return prs, nil
}

func (s *GitRepositoryService) getGitLabMergeRequest(ctx context.Context, repo *types.GitRepository, mrIID int) (*types.PullRequest, error) {
	client, err := s.getGitLabClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	projectID, err := s.getGitLabProjectID(ctx, client, repo)
	if err != nil {
		return nil, err
	}

	glMR, err := client.GetMergeRequest(ctx, projectID, mrIID)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}

	pr := &types.PullRequest{
		ID:           strconv.Itoa(glMR.IID),
		Number:       glMR.IID,
		Title:        glMR.Title,
		Description:  glMR.Description,
		State:        glMR.State,
		SourceBranch: glMR.SourceBranch,
		TargetBranch: glMR.TargetBranch,
		URL:          glMR.WebURL,
	}

	if glMR.Author != nil {
		pr.Author = glMR.Author.Username
	}

	if glMR.CreatedAt != nil {
		pr.CreatedAt = *glMR.CreatedAt
	}

	if glMR.UpdatedAt != nil {
		pr.UpdatedAt = *glMR.UpdatedAt
	}

	return pr, nil
}

// Bitbucket Pull Request Operations

func (s *GitRepositoryService) getBitbucketClient(ctx context.Context, repo *types.GitRepository) (*bitbucket.Client, error) {
	// Determine base URL (empty for bitbucket.org, custom for Bitbucket Server)
	var baseURL string
	var username string
	var appPassword string

	if repo.Bitbucket != nil {
		baseURL = repo.Bitbucket.BaseURL
		username = repo.Bitbucket.Username
		appPassword = repo.Bitbucket.AppPassword
	}

	// Fall back to generic username/password if Bitbucket-specific settings not available
	if username == "" {
		username = repo.Username
	}
	if appPassword == "" {
		appPassword = repo.Password
	}

	if username == "" || appPassword == "" {
		return nil, fmt.Errorf("no Bitbucket authentication configured - provide username and app password")
	}

	return bitbucket.NewClient(username, appPassword, baseURL), nil
}

func (s *GitRepositoryService) createBitbucketPullRequest(ctx context.Context, repo *types.GitRepository, title string, description string, sourceBranch string, targetBranch string) (string, error) {
	client, err := s.getBitbucketClient(ctx, repo)
	if err != nil {
		return "", err
	}

	workspace, repoSlug, _, err := bitbucket.ParseBitbucketURL(repo.ExternalURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse Bitbucket URL: %w", err)
	}

	pr, err := client.CreatePullRequest(ctx, workspace, repoSlug, title, description, sourceBranch, targetBranch)
	if err != nil {
		log.Error().Err(err).
			Str("workspace", workspace).
			Str("repo_slug", repoSlug).
			Str("title", title).
			Str("source_branch", sourceBranch).
			Str("target_branch", targetBranch).
			Msg("failed to create Bitbucket pull request")
		return "", fmt.Errorf("failed to create pull request: %w", err)
	}

	return strconv.Itoa(pr.ID), nil
}

func (s *GitRepositoryService) listBitbucketPullRequests(ctx context.Context, repo *types.GitRepository) ([]*types.PullRequest, error) {
	client, err := s.getBitbucketClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	workspace, repoSlug, _, err := bitbucket.ParseBitbucketURL(repo.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Bitbucket URL: %w", err)
	}

	bbPRs, err := client.ListPullRequests(ctx, workspace, repoSlug)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	prs := make([]*types.PullRequest, 0, len(bbPRs))
	for _, bbPR := range bbPRs {
		pr := &types.PullRequest{
			ID:           strconv.Itoa(bbPR.ID),
			Number:       bbPR.ID,
			Title:        bbPR.Title,
			Description:  bbPR.Description,
			State:        bbPR.State,
			SourceBranch: bbPR.SourceBranch,
			TargetBranch: bbPR.TargetBranch,
			Author:       bbPR.Author,
			URL:          bbPR.HTMLURL,
			CreatedAt:    bbPR.CreatedAt,
			UpdatedAt:    bbPR.UpdatedAt,
		}

		prs = append(prs, pr)
	}

	return prs, nil
}

func (s *GitRepositoryService) getBitbucketPullRequest(ctx context.Context, repo *types.GitRepository, prID int) (*types.PullRequest, error) {
	client, err := s.getBitbucketClient(ctx, repo)
	if err != nil {
		return nil, err
	}

	workspace, repoSlug, _, err := bitbucket.ParseBitbucketURL(repo.ExternalURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Bitbucket URL: %w", err)
	}

	bbPR, err := client.GetPullRequest(ctx, workspace, repoSlug, prID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	return &types.PullRequest{
		ID:           strconv.Itoa(bbPR.ID),
		Number:       bbPR.ID,
		Title:        bbPR.Title,
		Description:  bbPR.Description,
		State:        bbPR.State,
		SourceBranch: bbPR.SourceBranch,
		TargetBranch: bbPR.TargetBranch,
		Author:       bbPR.Author,
		URL:          bbPR.HTMLURL,
		CreatedAt:    bbPR.CreatedAt,
		UpdatedAt:    bbPR.UpdatedAt,
	}, nil
}
