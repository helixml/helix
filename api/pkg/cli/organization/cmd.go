package organization

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
	Use:     "organization",
	Short:   "Manage organizations",
	Long:    `Commands for managing organizations in Helix.`,
	Aliases: []string{"orgs", "org"},
	RunE: func(cmd *cobra.Command, args []string) error {
		// By default run the list command
		return listCmd.RunE(cmd, args)
	},
}

// GetRootCmd returns the root command for organizations
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
