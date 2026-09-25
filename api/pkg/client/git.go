package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
)

// WriteGitFile creates or updates a file in a git repository branch.
func (c *HelixClient) WriteGitFile(ctx context.Context, repoID string, req *types.UpdateGitRepositoryFileContentsRequest) (*types.GitRepositoryFileResponse, error) {
	bts, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp types.GitRepositoryFileResponse
	err = c.makeRequest(ctx, http.MethodPut, fmt.Sprintf("/git/repositories/%s/contents", repoID), bytes.NewBuffer(bts), &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// ReadGitFile reads a file from a git repository branch.
func (c *HelixClient) ReadGitFile(ctx context.Context, repoID, path, branch string) (*types.GitRepositoryFileResponse, error) {
	q := url.Values{}
	q.Set("path", path)
	if branch != "" {
		q.Set("branch", branch)
	}

	var resp types.GitRepositoryFileResponse
	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/git/repositories/%s/contents?%s", repoID, q.Encode()), nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

// GitCloneURL returns an HTTP clone URL for a Helix-hosted repository with
// this client's API key embedded as the password. Keep it out of logs.
func (c *HelixClient) GitCloneURL(repoID string) (string, error) {
	u, err := url.Parse(strings.TrimSuffix(c.url, "/api/v1"))
	if err != nil {
		return "", err
	}
	u.User = url.UserPassword("api", c.apiKey)
	u.Path = "/git/" + repoID
	return u.String(), nil
}
