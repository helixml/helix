package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

func LookupOrganization(ctx context.Context, apiClient client.Client, orgRef string) (*types.Organization, error) {
	// If the reference doesn't start with org_ prefix, assume it's a name
	if strings.HasPrefix(orgRef, system.OrganizationPrefix) {

		// Get by ID
		organization, err := apiClient.GetOrganization(ctx, orgRef)
		if err != nil {
			return nil, fmt.Errorf("failed to get organization: %w", err)
		}
		return organization, nil
	}

	// List organizations to find the one with matching name
	organizations, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	for _, o := range organizations {
		if o.Name == orgRef || o.ID == orgRef {
			return o, nil
		}
	}

	return nil, fmt.Errorf("organization not found: %s", orgRef)
}

// ResolveOrganization resolves an organization flag to an org ID.
// If orgFlag is provided, it looks up the org by name or ID.
// If orgFlag is empty, it defaults to the user's first organization.
// Returns empty string if no org is found (backward-compatible).
func ResolveOrganization(ctx context.Context, apiClient client.Client, orgFlag string) (string, error) {
	if orgFlag != "" {
		org, err := LookupOrganization(ctx, apiClient, orgFlag)
		if err != nil {
			return "", err
		}
		return org.ID, nil
	}

	orgs, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return "", nil
	}
	if len(orgs) > 0 {
		return orgs[0].ID, nil
	}

	return "", nil
}

// LookupTeam looks up a team by name or ID in an organization
func LookupTeam(ctx context.Context, apiClient *client.HelixClient, orgID, ref string) (*types.Team, error) {
	if strings.HasPrefix(ref, system.TeamPrefix) {
		team, err := apiClient.GetTeam(ctx, orgID, ref)
		if err != nil {
			return nil, fmt.Errorf("failed to get team: %w", err)
		}
		return team, nil
	}

	teams, err := apiClient.ListTeams(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to list teams: %w", err)
	}

	for _, team := range teams {
		if team.Name == ref {
			return team, nil
		}
	}

	return nil, fmt.Errorf("team not found: %s", ref)
}
