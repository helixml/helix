package app

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/spf13/cobra"
)

// Define organization as a persistent flag
var organization string

func init() {
	rootCmd.PersistentFlags().StringVarP(&organization, "organization", "o", "", "Organization ID or name")
}

var rootCmd = &cobra.Command{
	Use:     "agent",
	Short:   "Helix agent management",
	Aliases: []string{"app", "apps", "agents", "a"},
	Long:    `TODO`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listCmd.RunE(cmd, args)
	},
}

func New() *cobra.Command {
	return rootCmd
}

func lookupApp(ctx context.Context, apiClient *client.HelixClient, organization, ref string) (*types.App, error) {
	filter := &client.AppFilter{}

	if organization != "" {
		org, err := cli.LookupOrganization(ctx, apiClient, organization)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup organization: %w", err)
		}
		filter.OrganizationID = org.ID
	}

	apps, err := apiClient.ListApps(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	for _, app := range apps {
		if app.Config.Helix.Name == ref || app.ID == ref {
			return app, nil
		}
	}

	return nil, fmt.Errorf("agent not found: %s", ref)
}
