package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

// ListTeamMembers lists all members of a team
func (c *HelixClient) ListTeamMembers(ctx context.Context, organizationID, teamID string) ([]*types.TeamMembership, error) {
	url := fmt.Sprintf("/organizations/%s/teams/%s/members", organizationID, teamID)

	var members []*types.TeamMembership
	err := c.makeRequest(ctx, http.MethodGet, url, nil, &members)
	if err != nil {
		return nil, fmt.Errorf("error listing team members: %w", err)
	}

	return members, nil
}

// AddTeamMember adds a new member to a team
func (c *HelixClient) AddTeamMember(ctx context.Context, organizationID, teamID string, req *types.AddTeamMemberRequest) (*types.TeamMembership, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("/organizations/%s/teams/%s/members", organizationID, teamID)

	var membership *types.TeamMembership
	err = c.makeRequest(ctx, http.MethodPost, url, bytes.NewReader(body), &membership)
	if err != nil {
		return nil, fmt.Errorf("error adding team member: %w", err)
	}

	return membership, nil
}

// RemoveTeamMember removes a member from a team
func (c *HelixClient) RemoveTeamMember(ctx context.Context, organizationID, teamID, userID string) error {
	url := fmt.Sprintf("/organizations/%s/teams/%s/members/%s", organizationID, teamID, userID)

	err := c.makeRequest(ctx, http.MethodDelete, url, nil, nil)
	if err != nil {
		return fmt.Errorf("error removing team member: %w", err)
	}

	return nil
}
