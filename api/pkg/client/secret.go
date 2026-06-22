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

// SecretFilter filters the ListSecrets result. Empty filter returns
// the caller's personal secrets.
type SecretFilter struct {
	OrganizationID string
}

// ListSecrets retrieves a list of secrets, optionally scoped to an organization.
// Pass nil for personal secrets.
func (c *HelixClient) ListSecrets(ctx context.Context, f *SecretFilter) ([]*types.Secret, error) {
	path := "/secrets"
	if f != nil && f.OrganizationID != "" {
		path += "?organization_id=" + url.QueryEscape(f.OrganizationID)
	}

	var secrets []*types.Secret
	err := c.makeRequest(ctx, http.MethodGet, path, nil, &secrets)
	if err != nil {
		return nil, err
	}
	return secrets, nil
}

// CreateSecret creates a new secret
func (c *HelixClient) CreateSecret(ctx context.Context, secret *types.CreateSecretRequest) (*types.Secret, error) {
	var createdSecret types.Secret

	bts, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPost, "/secrets", bytes.NewBuffer(bts), &createdSecret)
	if err != nil {
		return nil, err
	}
	return &createdSecret, nil
}

// UpdateSecret updates an existing secret
func (c *HelixClient) UpdateSecret(ctx context.Context, id string, secret *types.Secret) (*types.Secret, error) {
	var updatedSecret types.Secret

	bts, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	err = c.makeRequest(ctx, http.MethodPut, fmt.Sprintf("/secrets/%s", id), bytes.NewBuffer(bts), &updatedSecret)
	if err != nil {
		return nil, err
	}
	return &updatedSecret, nil
}

// DeleteSecret deletes a secret by ID
func (c *HelixClient) DeleteSecret(ctx context.Context, id string) error {
	return c.makeRequest(ctx, http.MethodDelete, fmt.Sprintf("/secrets/%s", id), nil, nil)
}

// CreateProjectSecret creates a secret scoped to a specific project.
func (c *HelixClient) CreateProjectSecret(ctx context.Context, projectID string, secret *types.CreateSecretRequest) (*types.Secret, error) {
	secret.ProjectID = projectID

	bts, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	var created types.Secret
	err = c.makeRequest(ctx, http.MethodPost, fmt.Sprintf("/projects/%s/secrets", projectID), bytes.NewBuffer(bts), &created)
	if err != nil {
		return nil, err
	}
	return &created, nil
}
