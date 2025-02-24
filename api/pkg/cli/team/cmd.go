package team

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
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
	organizations, err := apiClient.ListOrganizations(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list organizations: %w", err)
	}

	for _, organization := range organizations {
		if organization.Name == ref || organization.ID == ref {
			return organization, nil
		}
	}

	return nil, fmt.Errorf("organization not found: %s", ref)
}
