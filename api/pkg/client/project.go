package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/types"
)

// ApplyProject idempotently creates or updates a project from a declarative spec.
func (c *HelixClient) ApplyProject(ctx context.Context, req *types.ProjectApplyRequest) (*types.ProjectApplyResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var resp types.ProjectApplyResponse
	if err := c.makeRequest(ctx, "PUT", "/api/v1/projects/apply", bytes.NewReader(body), &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}
