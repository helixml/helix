package github

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
)

// Client wraps the GitHub API client
type Client struct {
	client *github.Client
	ctx    context.Context
}

// NewClientWithPAT creates a new GitHub client using a Personal Access Token
// For github.com - use NewClientWithPATAndBaseURL for GitHub Enterprise
func NewClientWithPAT(token string) *Client {
	return NewClientWithPATAndBaseURL(token, "")
}

// NewClientWithPATAndBaseURL creates a new GitHub client using a Personal Access Token
// with an optional base URL for GitHub Enterprise instances.
// If baseURL is empty, uses github.com.
func NewClientWithPATAndBaseURL(token, baseURL string) *Client {
	ctx := context.Background()
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	tc := oauth2.NewClient(ctx, ts)
	client := github.NewClient(tc)

	// Configure for GitHub Enterprise if base URL is provided
	if baseURL != "" {
		// WithEnterpriseURLs sets the base and upload URLs for GitHub Enterprise
		// The upload URL is typically the same as the base URL for GHE
		enterpriseClient, err := client.WithEnterpriseURLs(baseURL, baseURL)
		if err == nil {
			client = enterpriseClient
		}
		// If there's an error, fall back to the standard client
		// This shouldn't happen with valid URLs but provides a safe fallback
	}

	return &Client{
		client: client,
		ctx:    ctx,
	}
}

// NewClientWithOAuth creates a new GitHub client using an OAuth access token
// OAuth tokens and PATs use the same authentication mechanism for GitHub API
// For github.com - use NewClientWithOAuthAndBaseURL for GitHub Enterprise
func NewClientWithOAuth(accessToken string) *Client {
	return NewClientWithOAuthAndBaseURL(accessToken, "")
}

// NewClientWithOAuthAndBaseURL creates a new GitHub client using an OAuth access token
// with an optional base URL for GitHub Enterprise instances.
// If baseURL is empty, uses github.com.
func NewClientWithOAuthAndBaseURL(accessToken, baseURL string) *Client {
	return NewClientWithPATAndBaseURL(accessToken, baseURL)
}

// NewClientWithGitHubApp creates a new GitHub client using GitHub App authentication.
// This uses JWT to get an installation access token, providing service-to-service auth.
// appID is the GitHub App ID, installationID is the installation ID for the app on the org/repo,
// and privateKey is the PEM-encoded private key for JWT signing.
// baseURL is optional for GitHub Enterprise instances (empty for github.com).
func NewClientWithGitHubApp(appID, installationID int64, privateKey, baseURL string) (*Client, error) {
	// Create the GitHub App installation transport
	// This handles JWT generation and token refresh automatically
	itr, err := ghinstallation.New(http.DefaultTransport, appID, installationID, []byte(privateKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create GitHub App transport: %w", err)
	}

	// Configure base URL for GitHub Enterprise if needed
	if baseURL != "" {
		// For GHE, set the API endpoint
		itr.BaseURL = strings.TrimSuffix(baseURL, "/") + "/api/v3"
	}

	ctx := context.Background()
	client := github.NewClient(&http.Client{Transport: itr})

	// Configure for GitHub Enterprise if base URL is provided
	if baseURL != "" {
		enterpriseClient, err := client.WithEnterpriseURLs(baseURL, baseURL)
		if err == nil {
			client = enterpriseClient
		}
	}

	return &Client{
		client: client,
		ctx:    ctx,
	}, nil
}

// ListRepositories lists all repositories accessible to the authenticated user
func (c *Client) ListRepositories(ctx context.Context) ([]*github.Repository, error) {
	var allRepos []*github.Repository
	opt := &github.RepositoryListOptions{
		ListOptions: github.ListOptions{PerPage: 100},
		Sort:        "updated",
	}

	for {
		repos, resp, err := c.client.Repositories.List(ctx, "", opt)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}
		allRepos = append(allRepos, repos...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allRepos, nil
}

// CreatePullRequest creates a new pull request
func (c *Client) CreatePullRequest(ctx context.Context, owner, repo, title, body, head, base string) (*github.PullRequest, error) {
	pr := &github.NewPullRequest{
		Title: &title,
		Body:  &body,
		Head:  &head,
		Base:  &base,
	}

	pullRequest, _, err := c.client.PullRequests.Create(ctx, owner, repo, pr)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return pullRequest, nil
}

// GetPullRequest gets a pull request by number
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*github.PullRequest, error) {
	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	return pr, nil
}

// ListPullRequests lists pull requests for a repository
func (c *Client) ListPullRequests(ctx context.Context, owner, repo string) ([]*github.PullRequest, error) {
	var allPRs []*github.PullRequest
	opt := &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 100},
	}

	for {
		prs, resp, err := c.client.PullRequests.List(ctx, owner, repo, opt)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull requests: %w", err)
		}
		allPRs = append(allPRs, prs...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allPRs, nil
}

// GetRepository gets a repository by owner and name
func (c *Client) GetRepository(ctx context.Context, owner, repo string) (*github.Repository, error) {
	repository, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository: %w", err)
	}
	return repository, nil
}

// GetAuthenticatedUser gets the authenticated user's profile
func (c *Client) GetAuthenticatedUser(ctx context.Context) (*github.User, error) {
	user, _, err := c.client.Users.Get(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get authenticated user: %w", err)
	}
	return user, nil
}

// CheckRepositoryPermissions checks if the authenticated user has write/push access to a repository
// Returns the permissions object which includes admin, push, pull booleans
func (c *Client) CheckRepositoryPermissions(ctx context.Context, owner, repo string) (*github.Repository, error) {
	repository, _, err := c.client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return nil, fmt.Errorf("failed to get repository permissions: %w", err)
	}
	return repository, nil
}

// HasPushAccess checks if the authenticated user can push to a repository
func (c *Client) HasPushAccess(ctx context.Context, owner, repo string) (bool, error) {
	repository, err := c.CheckRepositoryPermissions(ctx, owner, repo)
	if err != nil {
		return false, err
	}
	// Check permissions - the Permissions field is only populated for authenticated users
	perms := repository.GetPermissions()
	if perms != nil {
		// Push permission means the user can push to the repository
		if push, ok := perms["push"]; ok && push {
			return true, nil
		}
		// Admin permission also implies push access
		if admin, ok := perms["admin"]; ok && admin {
			return true, nil
		}
	}
	return false, nil
}

// ForkRepository creates a fork of a repository to the authenticated user's account
// If organization is empty, forks to the user's personal account
// Returns the forked repository
func (c *Client) ForkRepository(ctx context.Context, owner, repo, organization string) (*github.Repository, error) {
	opts := &github.RepositoryCreateForkOptions{}
	if organization != "" {
		opts.Organization = organization
	}

	forkedRepo, _, err := c.client.Repositories.CreateFork(ctx, owner, repo, opts)
	if err != nil {
		// GitHub returns 202 Accepted when fork is in progress
		// go-github handles this, but the error might still be raised
		if _, ok := err.(*github.AcceptedError); ok {
			// Fork is in progress, return the repo anyway
			// The fork endpoint returns the repo even with 202
		} else {
			return nil, fmt.Errorf("failed to fork repository: %w", err)
		}
	}

	// Ensure we have valid repo data - this can happen if GitHub returns 202
	// but the response body doesn't contain the forked repo details
	if forkedRepo == nil {
		return nil, fmt.Errorf("fork request accepted but no repository data returned for %s/%s", owner, repo)
	}

	return forkedRepo, nil
}

// ForkExists checks if a fork of the given repo already exists in the user's account or organization
func (c *Client) ForkExists(ctx context.Context, owner, repo, forkOwner string) (*github.Repository, error) {
	// Try to get the forked repo directly
	forkedRepo, resp, err := c.client.Repositories.Get(ctx, forkOwner, repo)
	if err != nil {
		if resp != nil && resp.StatusCode == 404 {
			return nil, nil // Fork doesn't exist
		}
		return nil, fmt.Errorf("failed to check for existing fork: %w", err)
	}
	// Verify it's actually a fork of the expected repo
	if forkedRepo.Fork != nil && *forkedRepo.Fork {
		if forkedRepo.Parent != nil {
			parentOwner := forkedRepo.Parent.Owner.GetLogin()
			parentName := forkedRepo.Parent.GetName()
			if parentOwner == owner && parentName == repo {
				return forkedRepo, nil // This is a fork of the expected repo
			}
		}
	}
	return nil, nil // Repo exists but is not a fork of the expected repo
}

// ParseGitHubURL extracts owner and repo from a GitHub URL
// Supports formats:
//   - https://github.com/owner/repo
//   - https://github.com/owner/repo.git
//   - git@github.com:owner/repo.git
//   - https://github.enterprise.com/owner/repo (GitHub Enterprise)
//   - git@github.enterprise.com:owner/repo.git (GitHub Enterprise SSH)
func ParseGitHubURL(url string) (owner, repo string, err error) {
	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")

	// Handle SSH format (git@host:owner/repo)
	if strings.HasPrefix(url, "git@") {
		// Extract the path part after the colon
		colonIdx := strings.Index(url, ":")
		if colonIdx != -1 {
			path := url[colonIdx+1:]
			parts := strings.Split(path, "/")
			if len(parts) == 2 {
				return parts[0], parts[1], nil
			}
		}
		return "", "", fmt.Errorf("invalid GitHub SSH URL: %s", url)
	}

	// Handle HTTPS format (https://host/owner/repo)
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		// Remove the protocol prefix
		withoutProtocol := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
		// Split by / to get host and path parts
		parts := strings.SplitN(withoutProtocol, "/", 2)
		if len(parts) == 2 {
			pathParts := strings.Split(parts[1], "/")
			if len(pathParts) >= 2 {
				return pathParts[0], pathParts[1], nil
			}
		}
	}

	return "", "", fmt.Errorf("unrecognized GitHub URL format: %s", url)
}
