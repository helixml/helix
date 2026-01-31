package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListHelixModels(ctx context.Context, q *store.ListModelsQuery) ([]*types.Model, error) {
	var models []*types.Model

	queryParams := url.Values{}

	if q.Type != "" {
		queryParams.Set("type", q.Type.String())
	}
	if q.Runtime != "" {
		queryParams.Set("runtime", strings.ToLower(q.Runtime.String()))
	}
	if q.Name != "" {
		queryParams.Set("name", q.Name)
	}

	err := c.makeRequest(ctx, http.MethodGet, "/helix-models?"+queryParams.Encode(), nil, &models)
	if err != nil {
		return nil, err
	}
	return models, nil
}

// CreateHelixModel creates a new Helix model configuration.
func (c *HelixClient) CreateHelixModel(ctx context.Context, model *types.Model) (*types.Model, error) {
	var createdModel types.Model

	bts, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal model: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPost, "/helix-models", bytes.NewBuffer(bts), &createdModel)
	if err != nil {
		return nil, err
	}
	return &createdModel, nil
}

// UpdateHelixModel updates an existing Helix model configuration.
func (c *HelixClient) UpdateHelixModel(ctx context.Context, id string, model *types.Model) (*types.Model, error) {
	var updatedModel types.Model

	bts, err := json.Marshal(model)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal model: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPut, fmt.Sprintf("/helix-models/%s", id), bytes.NewBuffer(bts), &updatedModel)
	if err != nil {
		return nil, err
	}
	return &updatedModel, nil
}

// DeleteHelixModel deletes a Helix model configuration by ID.
func (c *HelixClient) DeleteHelixModel(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodDelete, fmt.Sprintf("/helix-models/%s", id), nil, nil)
	if err != nil {
		return err
	}
	return nil
}
