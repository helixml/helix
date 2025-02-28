package app

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringP("filename", "f", "", "Filename to apply")
	applyCmd.Flags().Bool("shared", false, "Shared application")
	applyCmd.Flags().Bool("global", false, "Global application")
	applyCmd.Flags().Bool("refresh-knowledge", false, "Refresh knowledge, re-index all knowledge for the app")
	applyCmd.Flags().StringVarP(&organization, "organization", "o", "", "Organization ID or name")
}

func NewApplyCmd() *cobra.Command {
	return applyCmd
}

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Create or update an application",
	Long:  `Create or update an application.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
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

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		refreshKnowledge, err := cmd.Flags().GetBool("refresh-knowledge")
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

		existingApps, err := apiClient.ListApps(cmd.Context(), &client.AppFilter{})
		if err != nil {
			return err
		}

		for _, existingApp := range existingApps {
			if existingApp.Config.Helix.Name == appConfig.Name {
				log.Debug().Msgf("Existing app (%s) found, updating...", appConfig.Name)
				err = updateApp(cmd.Context(), apiClient, existingApp, appConfig, shared, global)
				if err != nil {
					return err
				}

				if refreshKnowledge {
					knowledgeFilter := &client.KnowledgeFilter{
						AppID: existingApp.ID,
					}

					knowledge, err := apiClient.ListKnowledge(cmd.Context(), knowledgeFilter)
					if err != nil {
						return err
					}

					for _, knowledge := range knowledge {
						err = apiClient.RefreshKnowledge(cmd.Context(), knowledge.ID)
						if err != nil {
							return fmt.Errorf("failed to refresh knowledge %s (%s): %w", knowledge.ID, knowledge.Name, err)
						}
					}
				}

				return nil
			}
		}

		return createApp(cmd.Context(), apiClient, organization, appConfig, shared, global)
	},
}

func updateApp(ctx context.Context, apiClient client.Client, app *types.App, appConfig *types.AppHelixConfig, shared, global bool) error {
	app.Config.Helix = *appConfig
	app.Shared = shared
	app.Global = global

	app, err := apiClient.UpdateApp(ctx, app)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", app.ID)

	return nil
}

func createApp(ctx context.Context, apiClient client.Client, orgID string, appConfig *types.AppHelixConfig, shared, global bool) error {
	org, err := cli.LookupOrganization(ctx, apiClient, orgID)
	if err != nil {
		return err
	}

	app := &types.App{
		OrganizationID: org.ID,
		AppSource:      types.AppSourceHelix,
		Global:         global,
		Shared:         shared,
		Config: types.AppConfig{
			AllowedDomains: []string{}, // TODO: make configurable
			Helix:          *appConfig,
		},
	}

	app, err = apiClient.CreateApp(ctx, app)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", app.ID)

	return nil
}
