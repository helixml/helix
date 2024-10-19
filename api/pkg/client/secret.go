package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// ListSecrets retrieves a list of secrets
func (c *HelixClient) ListSecrets() ([]*types.Secret, error) {
	var secrets []*types.Secret
	err := c.makeRequest(http.MethodGet, "/secrets", nil, &secrets)
	if err != nil {
		return nil, fmt.Errorf("failed to list secrets: %w", err)
	}
	return secrets, nil
}

// CreateSecret creates a new secret
func (c *HelixClient) CreateSecret(secret *types.Secret) (*types.Secret, error) {
	var createdSecret types.Secret

	bts, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	err = c.makeRequest(http.MethodPost, "/secrets", bytes.NewBuffer(bts), &createdSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to create secret: %w", err)
	}
	return &createdSecret, nil
}

// UpdateSecret updates an existing secret
func (c *HelixClient) UpdateSecret(id string, secret *types.Secret) (*types.Secret, error) {
	var updatedSecret types.Secret

	bts, err := json.Marshal(secret)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal secret: %w", err)
	}

	err = c.makeRequest(http.MethodPut, fmt.Sprintf("/secrets/%s", id), bytes.NewBuffer(bts), &updatedSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to update secret: %w", err)
	}
	return &updatedSecret, nil
}

// DeleteSecret deletes a secret by ID
func (c *HelixClient) DeleteSecret(id string) error {
	err := c.makeRequest(http.MethodDelete, fmt.Sprintf("/secrets/%s", id), nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}
	return nil
}
