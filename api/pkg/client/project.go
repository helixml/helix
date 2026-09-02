package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/helixml/helix/api/pkg/types"
)

// GetProject returns one project by its canonical ID.
func (c *HelixClient) GetProject(ctx context.Context, projectID string) (*types.Project, error) {
	var project types.Project
	if err := c.makeRequest(ctx, http.MethodGet, "/projects/"+url.PathEscape(projectID), nil, &project); err != nil {
		return nil, err
	}
	return &project, nil
}

// ListProjects returns the caller's personal projects, or their visible
// projects in organizationID when it is set.
func (c *HelixClient) ListProjects(ctx context.Context, organizationID string) ([]*types.Project, error) {
	path := "/projects"
	if organizationID != "" {
		path += "?organization_id=" + url.QueryEscape(organizationID)
	}
	var projects []*types.Project
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &projects); err != nil {
		return nil, err
	}
	return projects, nil
}

// ApplyProject idempotently creates or updates a project from a declarative spec.
func (c *HelixClient) ApplyProject(ctx context.Context, req *types.ProjectApplyRequest) (*types.ProjectApplyResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp types.ProjectApplyResponse
	if err := c.makeRequest(ctx, "PUT", "/projects/apply", bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
