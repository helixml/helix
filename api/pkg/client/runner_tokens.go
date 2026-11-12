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

// CreateRunnerToken mints a per-runner token (admin only). The response is
// the only place the token secret ever appears.
func (c *HelixClient) CreateRunnerToken(ctx context.Context, req *types.CreateRunnerTokenRequest) (*types.CreateRunnerTokenResponse, error) {
	bts, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal runner token request: %w", err)
	}
	var created types.CreateRunnerTokenResponse
	if err := c.makeRequest(ctx, http.MethodPost, "/admin/runner-tokens", bytes.NewBuffer(bts), &created); err != nil {
		return nil, err
	}
	return &created, nil
}

// ListRunnerTokens lists per-runner tokens (admin only); an empty runnerID
// lists every runner's tokens.
func (c *HelixClient) ListRunnerTokens(ctx context.Context, runnerID string) ([]*types.RunnerToken, error) {
	path := "/admin/runner-tokens"
	if runnerID != "" {
		path += "?runner_id=" + url.QueryEscape(runnerID)
	}
	var tokens []*types.RunnerToken
	if err := c.makeRequest(ctx, http.MethodGet, path, nil, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

// RevokeRunnerToken revokes a per-runner token by id (admin only).
func (c *HelixClient) RevokeRunnerToken(ctx context.Context, id string) (*types.RunnerToken, error) {
	var revoked types.RunnerToken
	if err := c.makeRequest(ctx, http.MethodDelete, "/admin/runner-tokens/"+url.PathEscape(id), nil, &revoked); err != nil {
		return nil, err
	}
	return &revoked, nil
}
