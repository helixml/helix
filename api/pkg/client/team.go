package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/helixml/helix/api/pkg/types"
)

func (c *HelixClient) ListTeams(ctx context.Context, organizationID string) ([]*types.Team, error) {
	var teams []*types.Team
	err := c.makeRequest(ctx, http.MethodGet, "/organizations/"+organizationID+"/teams", nil, &teams)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}
	return teams, nil
}

func (c *HelixClient) GetTeam(ctx context.Context, organizationID, teamID string) (*types.Team, error) {
	var team *types.Team
	err := c.makeRequest(ctx, http.MethodGet, "/organizations/"+organizationID+"/teams/"+teamID, nil, &team)
	if err != nil {
		return nil, fmt.Errorf("failed to get team: %w", err)
	}
	return team, nil
}

func (c *HelixClient) CreateTeam(ctx context.Context, organizationID string, req *types.CreateTeamRequest) (*types.Team, error) {
	var team *types.Team
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal team request: %w", err)
	}
	err = c.makeRequest(ctx, http.MethodPost, "/organizations/"+organizationID+"/teams", bytes.NewReader(body), &team)
	if err != nil {
		return nil, fmt.Errorf("failed to create team: %w", err)
	}
	return team, nil
}

func (c *HelixClient) DeleteTeam(ctx context.Context, organizationID, teamID string) error {
	err := c.makeRequest(ctx, http.MethodDelete, "/organizations/"+organizationID+"/teams/"+teamID, nil, nil)
	if err != nil {
		return fmt.Errorf("failed to delete team: %w", err)
	}
	return nil
}

// PrettyPrintTeam prints a team struct as pretty JSON
func (c *HelixClient) PrettyPrintTeam(team *types.Team) error {
	data, err := json.MarshalIndent(team, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal team: %w", err)
	}
	fmt.Println(string(data))
	return nil
}
