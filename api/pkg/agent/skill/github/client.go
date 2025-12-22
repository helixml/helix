package github

import (
	"context"
	"fmt"
	"strings"

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
