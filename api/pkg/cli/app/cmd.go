package app

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:     "app",
	Short:   "Helix app management",
	Aliases: []string{"a"},
	Long:    `TODO`,
	Run: func(cmd *cobra.Command, args []string) {
		// Do Stuff Here
	},
}

func New() *cobra.Command {
	return rootCmd
}

func lookupApp(ctx context.Context, apiClient *client.HelixClient, ref string) (*types.App, error) {
	apps, err := apiClient.ListApps(ctx, &client.AppFilter{})
	if err != nil {
		return nil, fmt.Errorf("failed to list apps: %w", err)
	}

	for _, app := range apps {
		if app.Config.Helix.Name == ref || app.ID == ref {
			return app, nil
		}
	}

	return nil, fmt.Errorf("app not found: %s", ref)
}
