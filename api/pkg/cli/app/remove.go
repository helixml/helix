package app

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Delete helix app",
	Long:    ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("app name or ID is required")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		apps, err := apiClient.ListApps(&client.AppFilter{})
		if err != nil {
			return fmt.Errorf("failed to list apps: %w", err)
		}

		// Find the app by name or ID
		var app *types.App
		for _, a := range apps {
			if a.Config.Helix.Name == args[0] || a.ID == args[0] {
				app = a
				break
			}
		}

		if app == nil {
			return fmt.Errorf("app %s not found", args[0])
		}

		// Delete the app
		if err := apiClient.DeleteApp(app.ID); err != nil {
			return fmt.Errorf("failed to delete app: %w", err)
		}

		log.Info().Msgf("App %s deleted", app.ID)

		return nil
	},
}
