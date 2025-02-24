package team

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
	Use:     "team",
	Short:   "Manage teams",
	Long:    `Commands for managing teams in Helix.`,
	Aliases: []string{"teams"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the list command
		return listCmd.RunE(cmd, args)
	},
}

// New returns the root command for teams
func New() *cobra.Command {
	return rootCmd
}

func lookupOrganization(ctx context.Context, apiClient *client.HelixClient, ref string) (*types.Organization, error) {

	// If the reference doesn't start with org_ prefix, assume it's a name
	if strings.HasPrefix(ref, system.OrganizationPrefix) {
		// Get by ID
		organization, err := apiClient.GetOrganization(ctx, ref)
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

	found := false
	for _, org := range organizations {
		if org.Name == ref {
			ref = org.ID
			found = true
			break
		}
	}

	if !found {
		return nil, fmt.Errorf("organization not found: %s", ref)
	}

	return nil, fmt.Errorf("organization not found: %s", ref)
}
