package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// ListProviderEndpoints retrieves a list of provider endpoints for the authenticated user
func (c *HelixClient) ListProviderEndpoints(ctx context.Context) ([]*types.ProviderEndpoint, error) {
	var endpoints []*types.ProviderEndpoint
	err := c.makeRequest(ctx, http.MethodGet, "/providers-endpoints", nil, &endpoints)
	if err != nil {
		return nil, fmt.Errorf("failed to list provider endpoints: %w", err)
	}
	return endpoints, nil
}

// GetProviderEndpoint retrieves a specific provider endpoint by ID
func (c *HelixClient) GetProviderEndpoint(ctx context.Context, id string) (*types.ProviderEndpoint, error) {
	var endpoint *types.ProviderEndpoint
	err := c.makeRequest(ctx, http.MethodGet, fmt.Sprintf("/providers-endpoints/%s", id), nil, &endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider endpoint: %w", err)
	}
	return endpoint, nil
}

// CreateProviderEndpoint creates a new provider endpoint
func (c *HelixClient) CreateProviderEndpoint(ctx context.Context, endpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	var createdEndpoint *types.ProviderEndpoint

	bts, err := json.Marshal(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal endpoint: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPost, "/providers-endpoints", bytes.NewBuffer(bts), &createdEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create provider endpoint: %w", err)
	}
	return createdEndpoint, nil
}

// UpdateProviderEndpoint updates an existing provider endpoint
func (c *HelixClient) UpdateProviderEndpoint(ctx context.Context, endpoint *types.ProviderEndpoint) (*types.ProviderEndpoint, error) {
	var updatedEndpoint *types.ProviderEndpoint

	bts, err := json.Marshal(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal endpoint: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPut, fmt.Sprintf("/providers-endpoints/%s", endpoint.ID), bytes.NewBuffer(bts), &updatedEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to update provider endpoint: %w", err)
	}
	return updatedEndpoint, nil
}

// DeleteProviderEndpoint deletes a provider endpoint by ID
func (c *HelixClient) DeleteProviderEndpoint(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodDelete, fmt.Sprintf("/providers-endpoints/%s", id), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete provider endpoint: %w", err)
	}
	return nil
}
