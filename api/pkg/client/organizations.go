package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListOrganizations(ctx context.Context) ([]*types.Organization, error) {
	var organizations []*types.Organization
	err := c.makeRequest(ctx, http.MethodGet, "/organizations", nil, &organizations)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}
	return organizations, nil
}

func (c *HelixClient) GetOrganization(ctx context.Context, reference string) (*types.Organization, error) {
	var organization *types.Organization
	err := c.makeRequest(ctx, http.MethodGet, "/organizations/"+reference, nil, &organization)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return organization, nil
}

func (c *HelixClient) CreateOrganization(ctx context.Context, organization *types.Organization) (*types.Organization, error) {
	var createdOrg *types.Organization
	body, err := json.Marshal(organization)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal organization: %w", err)
	}
	err = c.makeRequest(ctx, http.MethodPost, "/organizations", bytes.NewReader(body), &createdOrg)
	if err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}
	return createdOrg, nil
}

func (c *HelixClient) UpdateOrganization(ctx context.Context, id string, organization *types.Organization) (*types.Organization, error) {
	var updatedOrg *types.Organization
	body, err := json.Marshal(organization)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal organization: %w", err)
	}
	err = c.makeRequest(ctx, http.MethodPut, "/organizations/"+id, bytes.NewReader(body), &updatedOrg)
	if err != nil {
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}
	return updatedOrg, nil
}

func (c *HelixClient) DeleteOrganization(ctx context.Context, id string) error {
	err := c.makeRequest(ctx, http.MethodDelete, "/organizations/"+id, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}
	return nil
}
