package gitlab

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/xanzy/go-gitlab"
)

// Client wraps the GitLab API client
type Client struct {
	client *gitlab.Client
}

// NewClientWithPAT creates a new GitLab client using a Personal Access Token
// baseURL is optional - leave empty for gitlab.com, or provide for self-hosted GitLab
func NewClientWithPAT(baseURL, token string) (*Client, error) {
	var client *gitlab.Client
	var err error

	if baseURL != "" {
		client, err = gitlab.NewClient(token, gitlab.WithBaseURL(baseURL))
	} else {
		client, err = gitlab.NewClient(token)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{client: client}, nil
}

// NewClientWithOAuth creates a new GitLab client using an OAuth access token
// baseURL is optional - leave empty for gitlab.com, or provide for self-hosted GitLab
func NewClientWithOAuth(baseURL, accessToken string) (*Client, error) {
	var client *gitlab.Client
	var err error

	if baseURL != "" {
		client, err = gitlab.NewOAuthClient(accessToken, gitlab.WithBaseURL(baseURL))
	} else {
		client, err = gitlab.NewOAuthClient(accessToken)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab OAuth client: %w", err)
	}

	return &Client{client: client}, nil
}

// ListProjects lists all projects accessible to the authenticated user
func (c *Client) ListProjects(ctx context.Context) ([]*gitlab.Project, error) {
	var allProjects []*gitlab.Project
	opt := &gitlab.ListProjectsOptions{
		ListOptions: gitlab.ListOptions{PerPage: 100},
		Membership:  gitlab.Ptr(true),
		OrderBy:     gitlab.Ptr("updated_at"),
	}

	for {
		projects, resp, err := c.client.Projects.ListProjects(opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to list projects: %w", err)
		}
		allProjects = append(allProjects, projects...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allProjects, nil
}

// CreateMergeRequest creates a new merge request
func (c *Client) CreateMergeRequest(ctx context.Context, projectID int, title, description, sourceBranch, targetBranch string) (*gitlab.MergeRequest, error) {
	opt := &gitlab.CreateMergeRequestOptions{
		Title:        &title,
		Description:  &description,
		SourceBranch: &sourceBranch,
		TargetBranch: &targetBranch,
	}

	mr, _, err := c.client.MergeRequests.CreateMergeRequest(projectID, opt, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	return mr, nil
}

// GetMergeRequest gets a merge request by IID
func (c *Client) GetMergeRequest(ctx context.Context, projectID, mrIID int) (*gitlab.MergeRequest, error) {
	mr, _, err := c.client.MergeRequests.GetMergeRequest(projectID, mrIID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}
	return mr, nil
}

// ListMergeRequests lists merge requests for a project
func (c *Client) ListMergeRequests(ctx context.Context, projectID int) ([]*gitlab.MergeRequest, error) {
	var allMRs []*gitlab.MergeRequest
	state := "opened"
	opt := &gitlab.ListProjectMergeRequestsOptions{
		State:       &state,
		ListOptions: gitlab.ListOptions{PerPage: 100},
	}

	for {
		mrs, resp, err := c.client.MergeRequests.ListProjectMergeRequests(projectID, opt, gitlab.WithContext(ctx))
		if err != nil {
			return nil, fmt.Errorf("failed to list merge requests: %w", err)
		}
		allMRs = append(allMRs, mrs...)
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allMRs, nil
}

// GetProject gets a project by ID
func (c *Client) GetProject(ctx context.Context, projectID int) (*gitlab.Project, error) {
	project, _, err := c.client.Projects.GetProject(projectID, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return project, nil
}

// GetProjectByPath gets a project by its path (e.g., "group/project")
func (c *Client) GetProjectByPath(ctx context.Context, path string) (*gitlab.Project, error) {
	project, _, err := c.client.Projects.GetProject(path, nil, gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}
	return project, nil
}

// GetCurrentUser gets the authenticated user's profile
func (c *Client) GetCurrentUser(ctx context.Context) (*gitlab.User, error) {
	user, _, err := c.client.Users.CurrentUser(gitlab.WithContext(ctx))
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	return user, nil
}

// ParseGitLabURL extracts the project path or ID from a GitLab URL
// Supports formats:
//   - https://gitlab.com/group/project
//   - https://gitlab.com/group/subgroup/project
//   - https://gitlab.com/group/project.git
//   - git@gitlab.com:group/project.git
//   - https://self-hosted.gitlab.example.com/group/project
func ParseGitLabURL(url string) (baseURL, projectPath string, err error) {
	// Remove trailing .git
	url = strings.TrimSuffix(url, ".git")

	// Handle SSH format
	if strings.HasPrefix(url, "git@") {
		// git@gitlab.com:group/project -> gitlab.com, group/project
		re := regexp.MustCompile(`git@([^:]+):(.+)`)
		matches := re.FindStringSubmatch(url)
		if len(matches) == 3 {
			host := matches[1]
			path := matches[2]
			if host == "gitlab.com" {
				return "", path, nil
			}
			return "https://" + host, path, nil
		}
		return "", "", fmt.Errorf("invalid GitLab SSH URL: %s", url)
	}

	// Handle HTTPS format
	re := regexp.MustCompile(`https?://([^/]+)/(.+)`)
	matches := re.FindStringSubmatch(url)
	if len(matches) == 3 {
		host := matches[1]
		path := matches[2]
		if host == "gitlab.com" {
			return "", path, nil
		}
		return "https://" + host, path, nil
	}

	return "", "", fmt.Errorf("unrecognized GitLab URL format: %s", url)
}

// ParseProjectID attempts to parse a project identifier which could be:
// - A numeric ID (e.g., "12345")
// - A path (e.g., "group/project")
func ParseProjectID(identifier string) (int, string, error) {
	// Try parsing as integer
	if id, err := strconv.Atoi(identifier); err == nil {
		return id, "", nil
	}
	// Return as path
	return 0, identifier, nil
}
