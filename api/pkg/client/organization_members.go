package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// ListOrganizationMembers lists all members of an organization
func (c *HelixClient) ListOrganizationMembers(ctx context.Context, organizationID string) ([]*types.OrganizationMembership, error) {
	url := fmt.Sprintf("/organizations/%s/members", organizationID)

	var members []*types.OrganizationMembership
	err := c.makeRequest(ctx, http.MethodGet, url, nil, &members)
	if err != nil {
		return nil, fmt.Errorf("error listing organization members: %w", err)
	}

	return members, nil
}

// AddOrganizationMember adds a new member to an organization
func (c *HelixClient) AddOrganizationMember(ctx context.Context, organizationID string, req *types.AddOrganizationMemberRequest) (*types.OrganizationMembership, error) {
	url := fmt.Sprintf("/organizations/%s/members", organizationID)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var membership *types.OrganizationMembership
	err = c.makeRequest(ctx, http.MethodPost, url, bytes.NewReader(body), &membership)
	if err != nil {
		return nil, fmt.Errorf("error adding organization member: %w", err)
	}

	return membership, nil
}

// UpdateOrganizationMember updates a member's role in an organization
func (c *HelixClient) UpdateOrganizationMember(ctx context.Context, organizationID, userID string, req *types.UpdateOrganizationMemberRequest) (*types.OrganizationMembership, error) {
	url := fmt.Sprintf("/organizations/%s/members/%s", organizationID, userID)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	var membership *types.OrganizationMembership
	err = c.makeRequest(ctx, http.MethodPut, url, bytes.NewReader(body), &membership)
	if err != nil {
		return nil, fmt.Errorf("error updating organization member: %w", err)
	}

	return membership, nil
}

// RemoveOrganizationMember removes a member from an organization
func (c *HelixClient) RemoveOrganizationMember(ctx context.Context, organizationID, userID string) error {
	url := fmt.Sprintf("/organizations/%s/members/%s", organizationID, userID)

	err := c.makeRequest(ctx, http.MethodDelete, url, nil, nil)
	if err != nil {
		return fmt.Errorf("error removing organization member: %w", err)
	}

	return nil
}
