package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListOrganizationRoles(ctx context.Context, organizationID string) ([]*types.Role, error) {
	var roles []*types.Role
	err := c.makeRequest(ctx, http.MethodGet, "/organizations/"+organizationID+"/roles", nil, &roles)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}
	return roles, nil
}
