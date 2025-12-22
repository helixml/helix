package bitbucket

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

const (
	// Bitbucket Cloud API base URL
	cloudAPIBaseURL = "https://api.bitbucket.org/2.0"
)

// Client wraps the Bitbucket API client
type Client struct {
	httpClient  *http.Client
	username    string
	appPassword string
	baseURL     string // Empty for cloud, custom URL for server
	isServer    bool   // True if using Bitbucket Server/Data Center
}

// NewClient creates a new Bitbucket client
// For Bitbucket Cloud: use username + app password, leave baseURL empty
// For Bitbucket Server: use username + personal access token, set baseURL to server URL
func NewClient(username, appPassword, baseURL string) *Client {
	isServer := baseURL != "" && !strings.Contains(baseURL, "bitbucket.org")

	return &Client{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		username:    username,
		appPassword: appPassword,
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		isServer:    isServer,
	}
}

// getAPIURL returns the appropriate API URL based on cloud vs server
func (c *Client) getAPIURL(path string) string {
	if c.isServer {
		// Bitbucket Server uses /rest/api/1.0 prefix
		return fmt.Sprintf("%s/rest/api/1.0%s", c.baseURL, path)
	}
	return fmt.Sprintf("%s%s", cloudAPIBaseURL, path)
}

// doRequest performs an authenticated HTTP request
func (c *Client) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.SetBasicAuth(c.username, c.appPassword)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// UserProfile represents a Bitbucket user
type UserProfile struct {
	UUID        string `json:"uuid"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email,omitempty"`
}

// GetCurrentUser returns the authenticated user's profile
func (c *Client) GetCurrentUser(ctx context.Context) (*UserProfile, error) {
	var apiURL string
	if c.isServer {
		// Bitbucket Server: /rest/api/1.0/users/{username} or application-properties
		apiURL = c.getAPIURL("/users/" + c.username)
	} else {
		// Bitbucket Cloud
		apiURL = c.getAPIURL("/user")
	}

	resp, err := c.doRequest(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get current user: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get user: status %d, body: %s", resp.StatusCode, string(body))
	}

	if c.isServer {
		// Bitbucket Server response format
		var serverUser struct {
			ID          int    `json:"id"`
			Name        string `json:"name"`
			DisplayName string `json:"displayName"`
			Email       string `json:"emailAddress"`
			Slug        string `json:"slug"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&serverUser); err != nil {
			return nil, fmt.Errorf("failed to decode server user: %w", err)
		}
		return &UserProfile{
			UUID:        fmt.Sprintf("%d", serverUser.ID),
			Username:    serverUser.Name,
			DisplayName: serverUser.DisplayName,
			Email:       serverUser.Email,
		}, nil
	}

	// Bitbucket Cloud response
	var cloudUser UserProfile
	if err := json.NewDecoder(resp.Body).Decode(&cloudUser); err != nil {
		return nil, fmt.Errorf("failed to decode cloud user: %w", err)
	}
	return &cloudUser, nil
}

// Repository represents a Bitbucket repository
type Repository struct {
	UUID        string `json:"uuid"`
	Name        string `json:"name"`
	FullName    string `json:"full_name"`
	Slug        string `json:"slug"`
	Description string `json:"description"`
	IsPrivate   bool   `json:"is_private"`
	CloneURL    string `json:"clone_url"`
	HTMLURL     string `json:"html_url"`
	MainBranch  string `json:"mainbranch"`
	Project     string `json:"project"`
}

// ListRepositories lists all repositories accessible to the authenticated user
func (c *Client) ListRepositories(ctx context.Context) ([]Repository, error) {
	if c.isServer {
		return c.listServerRepositories(ctx)
	}
	return c.listCloudRepositories(ctx)
}

func (c *Client) listCloudRepositories(ctx context.Context) ([]Repository, error) {
	var allRepos []Repository
	nextURL := c.getAPIURL("/user/permissions/repositories?pagelen=100")

	for nextURL != "" {
		resp, err := c.doRequest(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list repositories: status %d, body: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Values []struct {
				Repository struct {
					UUID     string `json:"uuid"`
					Name     string `json:"name"`
					FullName string `json:"full_name"`
					Slug     string `json:"slug"`
					IsPrivate bool   `json:"is_private"`
					Links    struct {
						HTML  struct{ Href string } `json:"html"`
						Clone []struct {
							Name string `json:"name"`
							Href string `json:"href"`
						} `json:"clone"`
					} `json:"links"`
					MainBranch *struct {
						Name string `json:"name"`
					} `json:"mainbranch"`
					Description string `json:"description"`
				} `json:"repository"`
			} `json:"values"`
			Next string `json:"next"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode repositories: %w", err)
		}

		for _, v := range result.Values {
			repo := v.Repository
			cloneURL := ""
			for _, link := range repo.Links.Clone {
				if link.Name == "https" {
					cloneURL = link.Href
					break
				}
			}
			mainBranch := "main"
			if repo.MainBranch != nil {
				mainBranch = repo.MainBranch.Name
			}

			allRepos = append(allRepos, Repository{
				UUID:        repo.UUID,
				Name:        repo.Name,
				FullName:    repo.FullName,
				Slug:        repo.Slug,
				Description: repo.Description,
				IsPrivate:   repo.IsPrivate,
				CloneURL:    cloneURL,
				HTMLURL:     repo.Links.HTML.Href,
				MainBranch:  mainBranch,
			})
		}

		nextURL = result.Next
	}

	return allRepos, nil
}

func (c *Client) listServerRepositories(ctx context.Context) ([]Repository, error) {
	var allRepos []Repository
	start := 0
	limit := 100

	for {
		apiURL := fmt.Sprintf("%s?start=%d&limit=%d", c.getAPIURL("/repos"), start, limit)
		resp, err := c.doRequest(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list repositories: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list repositories: status %d, body: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Values []struct {
				ID      int    `json:"id"`
				Name    string `json:"name"`
				Slug    string `json:"slug"`
				Public  bool   `json:"public"`
				Project struct {
					Key  string `json:"key"`
					Name string `json:"name"`
				} `json:"project"`
				Links struct {
					Clone []struct {
						Name string `json:"name"`
						Href string `json:"href"`
					} `json:"clone"`
					Self []struct {
						Href string `json:"href"`
					} `json:"self"`
				} `json:"links"`
			} `json:"values"`
			IsLastPage bool `json:"isLastPage"`
			NextPageStart int `json:"nextPageStart"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode repositories: %w", err)
		}

		for _, repo := range result.Values {
			cloneURL := ""
			htmlURL := ""
			for _, link := range repo.Links.Clone {
				if link.Name == "http" || link.Name == "https" {
					cloneURL = link.Href
					break
				}
			}
			if len(repo.Links.Self) > 0 {
				htmlURL = repo.Links.Self[0].Href
			}

			fullName := fmt.Sprintf("%s/%s", repo.Project.Key, repo.Slug)

			allRepos = append(allRepos, Repository{
				UUID:        fmt.Sprintf("%d", repo.ID),
				Name:        repo.Name,
				FullName:    fullName,
				Slug:        repo.Slug,
				Description: "",
				IsPrivate:   !repo.Public,
				CloneURL:    cloneURL,
				HTMLURL:     htmlURL,
				Project:     repo.Project.Key,
			})
		}

		if result.IsLastPage {
			break
		}
		start = result.NextPageStart
	}

	return allRepos, nil
}

// PullRequest represents a Bitbucket pull request
type PullRequest struct {
	ID           int       `json:"id"`
	Title        string    `json:"title"`
	Description  string    `json:"description"`
	State        string    `json:"state"`
	SourceBranch string    `json:"source_branch"`
	TargetBranch string    `json:"target_branch"`
	Author       string    `json:"author"`
	HTMLURL      string    `json:"html_url"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreatePullRequest creates a new pull request
func (c *Client) CreatePullRequest(ctx context.Context, workspace, repoSlug, title, description, sourceBranch, targetBranch string) (*PullRequest, error) {
	if c.isServer {
		return c.createServerPullRequest(ctx, workspace, repoSlug, title, description, sourceBranch, targetBranch)
	}
	return c.createCloudPullRequest(ctx, workspace, repoSlug, title, description, sourceBranch, targetBranch)
}

func (c *Client) createCloudPullRequest(ctx context.Context, workspace, repoSlug, title, description, sourceBranch, targetBranch string) (*PullRequest, error) {
	apiURL := c.getAPIURL(fmt.Sprintf("/repositories/%s/%s/pullrequests", workspace, repoSlug))

	payload := map[string]interface{}{
		"title":       title,
		"description": description,
		"source": map[string]interface{}{
			"branch": map[string]string{"name": sourceBranch},
		},
		"destination": map[string]interface{}{
			"branch": map[string]string{"name": targetBranch},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create pull request: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
		Description string `json:"description"`
		State string `json:"state"`
		Source struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"source"`
		Destination struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"destination"`
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
		Links struct {
			HTML struct{ Href string } `json:"html"`
		} `json:"links"`
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, result.CreatedOn)
	updatedAt, _ := time.Parse(time.RFC3339, result.UpdatedOn)

	return &PullRequest{
		ID:           result.ID,
		Title:        result.Title,
		Description:  result.Description,
		State:        result.State,
		SourceBranch: result.Source.Branch.Name,
		TargetBranch: result.Destination.Branch.Name,
		Author:       result.Author.DisplayName,
		HTMLURL:      result.Links.HTML.Href,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func (c *Client) createServerPullRequest(ctx context.Context, projectKey, repoSlug, title, description, sourceBranch, targetBranch string) (*PullRequest, error) {
	apiURL := c.getAPIURL(fmt.Sprintf("/projects/%s/repos/%s/pull-requests", projectKey, repoSlug))

	payload := map[string]interface{}{
		"title":       title,
		"description": description,
		"fromRef": map[string]interface{}{
			"id": "refs/heads/" + sourceBranch,
		},
		"toRef": map[string]interface{}{
			"id": "refs/heads/" + targetBranch,
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.doRequest(ctx, "POST", apiURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to create pull request: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		FromRef     struct {
			ID string `json:"id"`
		} `json:"fromRef"`
		ToRef struct {
			ID string `json:"id"`
		} `json:"toRef"`
		Author struct {
			User struct {
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"author"`
		Links struct {
			Self []struct{ Href string } `json:"self"`
		} `json:"links"`
		CreatedDate int64 `json:"createdDate"`
		UpdatedDate int64 `json:"updatedDate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	htmlURL := ""
	if len(result.Links.Self) > 0 {
		htmlURL = result.Links.Self[0].Href
	}

	return &PullRequest{
		ID:           result.ID,
		Title:        result.Title,
		Description:  result.Description,
		State:        result.State,
		SourceBranch: strings.TrimPrefix(result.FromRef.ID, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(result.ToRef.ID, "refs/heads/"),
		Author:       result.Author.User.DisplayName,
		HTMLURL:      htmlURL,
		CreatedAt:    time.UnixMilli(result.CreatedDate),
		UpdatedAt:    time.UnixMilli(result.UpdatedDate),
	}, nil
}

// ListPullRequests lists open pull requests for a repository
func (c *Client) ListPullRequests(ctx context.Context, workspace, repoSlug string) ([]*PullRequest, error) {
	if c.isServer {
		return c.listServerPullRequests(ctx, workspace, repoSlug)
	}
	return c.listCloudPullRequests(ctx, workspace, repoSlug)
}

func (c *Client) listCloudPullRequests(ctx context.Context, workspace, repoSlug string) ([]*PullRequest, error) {
	var allPRs []*PullRequest
	nextURL := c.getAPIURL(fmt.Sprintf("/repositories/%s/%s/pullrequests?state=OPEN&pagelen=50", workspace, repoSlug))

	for nextURL != "" {
		resp, err := c.doRequest(ctx, "GET", nextURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull requests: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list pull requests: status %d, body: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Values []struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				State       string `json:"state"`
				Source      struct {
					Branch struct{ Name string } `json:"branch"`
				} `json:"source"`
				Destination struct {
					Branch struct{ Name string } `json:"branch"`
				} `json:"destination"`
				Author struct {
					DisplayName string `json:"display_name"`
				} `json:"author"`
				Links struct {
					HTML struct{ Href string } `json:"html"`
				} `json:"links"`
				CreatedOn string `json:"created_on"`
				UpdatedOn string `json:"updated_on"`
			} `json:"values"`
			Next string `json:"next"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode pull requests: %w", err)
		}

		for _, pr := range result.Values {
			createdAt, _ := time.Parse(time.RFC3339, pr.CreatedOn)
			updatedAt, _ := time.Parse(time.RFC3339, pr.UpdatedOn)

			allPRs = append(allPRs, &PullRequest{
				ID:           pr.ID,
				Title:        pr.Title,
				Description:  pr.Description,
				State:        pr.State,
				SourceBranch: pr.Source.Branch.Name,
				TargetBranch: pr.Destination.Branch.Name,
				Author:       pr.Author.DisplayName,
				HTMLURL:      pr.Links.HTML.Href,
				CreatedAt:    createdAt,
				UpdatedAt:    updatedAt,
			})
		}

		nextURL = result.Next
	}

	return allPRs, nil
}

func (c *Client) listServerPullRequests(ctx context.Context, projectKey, repoSlug string) ([]*PullRequest, error) {
	var allPRs []*PullRequest
	start := 0
	limit := 50

	for {
		apiURL := fmt.Sprintf("%s?state=OPEN&start=%d&limit=%d",
			c.getAPIURL(fmt.Sprintf("/projects/%s/repos/%s/pull-requests", projectKey, repoSlug)),
			start, limit)

		resp, err := c.doRequest(ctx, "GET", apiURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to list pull requests: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("failed to list pull requests: status %d, body: %s", resp.StatusCode, string(body))
		}

		var result struct {
			Values []struct {
				ID          int    `json:"id"`
				Title       string `json:"title"`
				Description string `json:"description"`
				State       string `json:"state"`
				FromRef     struct {
					ID string `json:"id"`
				} `json:"fromRef"`
				ToRef struct {
					ID string `json:"id"`
				} `json:"toRef"`
				Author struct {
					User struct {
						DisplayName string `json:"displayName"`
					} `json:"user"`
				} `json:"author"`
				Links struct {
					Self []struct{ Href string } `json:"self"`
				} `json:"links"`
				CreatedDate int64 `json:"createdDate"`
				UpdatedDate int64 `json:"updatedDate"`
			} `json:"values"`
			IsLastPage    bool `json:"isLastPage"`
			NextPageStart int  `json:"nextPageStart"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode pull requests: %w", err)
		}

		for _, pr := range result.Values {
			htmlURL := ""
			if len(pr.Links.Self) > 0 {
				htmlURL = pr.Links.Self[0].Href
			}

			allPRs = append(allPRs, &PullRequest{
				ID:           pr.ID,
				Title:        pr.Title,
				Description:  pr.Description,
				State:        pr.State,
				SourceBranch: strings.TrimPrefix(pr.FromRef.ID, "refs/heads/"),
				TargetBranch: strings.TrimPrefix(pr.ToRef.ID, "refs/heads/"),
				Author:       pr.Author.User.DisplayName,
				HTMLURL:      htmlURL,
				CreatedAt:    time.UnixMilli(pr.CreatedDate),
				UpdatedAt:    time.UnixMilli(pr.UpdatedDate),
			})
		}

		if result.IsLastPage {
			break
		}
		start = result.NextPageStart
	}

	return allPRs, nil
}

// GetPullRequest gets a specific pull request by ID
func (c *Client) GetPullRequest(ctx context.Context, workspace, repoSlug string, prID int) (*PullRequest, error) {
	if c.isServer {
		return c.getServerPullRequest(ctx, workspace, repoSlug, prID)
	}
	return c.getCloudPullRequest(ctx, workspace, repoSlug, prID)
}

func (c *Client) getCloudPullRequest(ctx context.Context, workspace, repoSlug string, prID int) (*PullRequest, error) {
	apiURL := c.getAPIURL(fmt.Sprintf("/repositories/%s/%s/pullrequests/%d", workspace, repoSlug, prID))

	resp, err := c.doRequest(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get pull request: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		Source      struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"source"`
		Destination struct {
			Branch struct{ Name string } `json:"branch"`
		} `json:"destination"`
		Author struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
		Links struct {
			HTML struct{ Href string } `json:"html"`
		} `json:"links"`
		CreatedOn string `json:"created_on"`
		UpdatedOn string `json:"updated_on"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	createdAt, _ := time.Parse(time.RFC3339, result.CreatedOn)
	updatedAt, _ := time.Parse(time.RFC3339, result.UpdatedOn)

	return &PullRequest{
		ID:           result.ID,
		Title:        result.Title,
		Description:  result.Description,
		State:        result.State,
		SourceBranch: result.Source.Branch.Name,
		TargetBranch: result.Destination.Branch.Name,
		Author:       result.Author.DisplayName,
		HTMLURL:      result.Links.HTML.Href,
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func (c *Client) getServerPullRequest(ctx context.Context, projectKey, repoSlug string, prID int) (*PullRequest, error) {
	apiURL := c.getAPIURL(fmt.Sprintf("/projects/%s/repos/%s/pull-requests/%d", projectKey, repoSlug, prID))

	resp, err := c.doRequest(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get pull request: status %d, body: %s", resp.StatusCode, string(body))
	}

	var result struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		Description string `json:"description"`
		State       string `json:"state"`
		FromRef     struct {
			ID string `json:"id"`
		} `json:"fromRef"`
		ToRef struct {
			ID string `json:"id"`
		} `json:"toRef"`
		Author struct {
			User struct {
				DisplayName string `json:"displayName"`
			} `json:"user"`
		} `json:"author"`
		Links struct {
			Self []struct{ Href string } `json:"self"`
		} `json:"links"`
		CreatedDate int64 `json:"createdDate"`
		UpdatedDate int64 `json:"updatedDate"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode pull request: %w", err)
	}

	htmlURL := ""
	if len(result.Links.Self) > 0 {
		htmlURL = result.Links.Self[0].Href
	}

	return &PullRequest{
		ID:           result.ID,
		Title:        result.Title,
		Description:  result.Description,
		State:        result.State,
		SourceBranch: strings.TrimPrefix(result.FromRef.ID, "refs/heads/"),
		TargetBranch: strings.TrimPrefix(result.ToRef.ID, "refs/heads/"),
		Author:       result.Author.User.DisplayName,
		HTMLURL:      htmlURL,
		CreatedAt:    time.UnixMilli(result.CreatedDate),
		UpdatedAt:    time.UnixMilli(result.UpdatedDate),
	}, nil
}

// ParseBitbucketURL extracts workspace/project and repo slug from a Bitbucket URL
// Supports:
// - Cloud: https://bitbucket.org/{workspace}/{repo}
// - Server: https://{server}/projects/{project}/repos/{repo}
func ParseBitbucketURL(repoURL string) (workspace string, repoSlug string, isServer bool, err error) {
	u, err := url.Parse(repoURL)
	if err != nil {
		return "", "", false, fmt.Errorf("invalid URL: %w", err)
	}

	path := strings.Trim(u.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	parts := strings.Split(path, "/")

	// Check for Bitbucket Server format: /projects/{project}/repos/{repo}
	if len(parts) >= 4 && parts[0] == "projects" && parts[2] == "repos" {
		return parts[1], parts[3], true, nil
	}

	// Check for Bitbucket Server SCM format: /scm/{project}/{repo}
	if len(parts) >= 3 && parts[0] == "scm" {
		return parts[1], parts[2], true, nil
	}

	// Bitbucket Cloud format: /{workspace}/{repo}
	if len(parts) >= 2 {
		return parts[0], parts[1], false, nil
	}

	return "", "", false, fmt.Errorf("unable to parse Bitbucket URL: %s", repoURL)
}

// ToRepositoryInfo converts a Bitbucket Repository to a types.RepositoryInfo
func (r *Repository) ToRepositoryInfo() types.RepositoryInfo {
	return types.RepositoryInfo{
		Name:          r.Name,
		FullName:      r.FullName,
		CloneURL:      r.CloneURL,
		HTMLURL:       r.HTMLURL,
		Description:   r.Description,
		Private:       r.IsPrivate,
		DefaultBranch: r.MainBranch,
	}
}
