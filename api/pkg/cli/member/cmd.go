package member

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "member",
	Short:   "Manage members in organizations and members",
	Long:    `Commands for managing organization and team members in Helix.`,
	Aliases: []string{"members"},
}

// New returns the root command for teams
func New() *cobra.Command {
	return rootCmd
}

func lookupOrganization(ctx context.Context, apiClient *client.HelixClient, orgRef string) (*types.Organization, error) {
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

func lookupTeam(ctx context.Context, apiClient *client.HelixClient, orgID, ref string) (*types.Team, error) {
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
