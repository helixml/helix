package app

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(NewApplyCmd())

	applyCmd.Flags().StringP("filename", "f", "", "Filename to apply")
	applyCmd.Flags().Bool("shared", false, "Shared application")
	applyCmd.Flags().Bool("global", false, "Global application")
}

func NewApplyCmd() *cobra.Command {
	return applyCmd
}

type applyOptions struct {
	filename string
}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update an application",
	Long:  `Create or update an application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		filename, err := cmd.Flags().GetString("filename")
		if err != nil {
			return err
		}

		if filename == "" {
			return fmt.Errorf("filename is required")
		}

		shared, err := cmd.Flags().GetBool("shared")
		if err != nil {
			return err
		}

		global, err := cmd.Flags().GetBool("global")
		if err != nil {
			return err
		}

		localApp, err := apps.NewLocalApp(filename)
		if err != nil {
			return err
		}

		appConfig := localApp.GetAppConfig()

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		existingApps, err := apiClient.ListApps(&client.AppFilter{})
		if err != nil {
			return err
		}

		for _, existingApp := range existingApps {
			if existingApp.Config.Helix.Name == appConfig.Name {
				log.Info().Msgf("Existing app (%s) found, updating...", appConfig.Name)
				return updateApp(apiClient, existingApp, appConfig, shared, global)
			}
		}

		return createApp(apiClient, appConfig, shared, global)
	},
}

func updateApp(apiClient client.Client, app *types.App, appConfig *types.AppHelixConfig, shared, global bool) error {
	app.Config.Helix = *appConfig
	app.Shared = shared
	app.Global = global

	app, err := apiClient.UpdateApp(app)
	if err != nil {
		return err
	}

	log.Info().Msgf("Updated app %s", app.ID)

	return nil
}

func createApp(apiClient client.Client, appConfig *types.AppHelixConfig, shared, global bool) error {
	app := &types.App{
		AppSource: types.AppSourceHelix,
		Global:    global,
		Shared:    shared,
		Config: types.AppConfig{
			AllowedDomains: []string{}, // TODO: make configurable
			Helix:          *appConfig,
		},
	}

	app, err := apiClient.CreateApp(app)
	if err != nil {
		return err
	}

	log.Info().Msgf("Created app %s", app.ID)

	return nil
}
