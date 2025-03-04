package app

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/apps"
	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/cli/util"
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
	applyCmd.Flags().StringSlice("rsync", []string{}, "Sync local files to the filestore for knowledge sources. Format: ./local/path[:knowledge_name]. If knowledge_name is omitted, uses the first knowledge source. Can be specified multiple times.")
	applyCmd.Flags().Bool("delete", false, "When used with --rsync, delete files in filestore that don't exist locally (similar to rsync --delete)")
	applyCmd.Flags().Bool("wait-knowledge", false, "Wait for knowledge sources to be fully indexed before returning")
	applyCmd.Flags().Duration("knowledge-timeout", 5*time.Minute, "Timeout when waiting for knowledge indexing")
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

		syncFiles, err := cmd.Flags().GetStringSlice("rsync")
		if err != nil {
			return err
		}

		deleteExtraFiles, err := cmd.Flags().GetBool("delete")
		if err != nil {
			return err
		}

		waitKnowledge, err := cmd.Flags().GetBool("wait-knowledge")
		if err != nil {
			return err
		}

		knowledgeTimeout, err := cmd.Flags().GetDuration("knowledge-timeout")
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

		// Handle the --rsync flag to sync local files to filestore
		if len(syncFiles) > 0 {
			mappings, err := util.ParseSyncMappings(syncFiles, appConfig)
			if err != nil {
				return err
			}

			for _, mapping := range mappings {
				fmt.Printf("Syncing local directory '%s' to knowledge source '%s' (path: %s)\n",
					mapping.LocalDir, mapping.KnowledgeName, mapping.RemotePath)

				err = util.SyncLocalDirToFilestore(cmd.Context(), apiClient, mapping.LocalDir, mapping.RemotePath, deleteExtraFiles)
				if err != nil {
					return fmt.Errorf("failed to sync files for knowledge '%s': %w", mapping.KnowledgeName, err)
				}
			}
		}

		// Handle app creation or update
		existingApps, err := apiClient.ListApps(cmd.Context(), &client.AppFilter{})
		if err != nil {
			return err
		}

		var existingApp *types.App
		var appID string

		for _, app := range existingApps {
			if app.Config.Helix.Name == appConfig.Name {
				log.Debug().Msgf("Existing app (%s) found, updating...", appConfig.Name)
				existingApp = app
				appID = app.ID
				break
			}
		}

		if existingApp != nil {
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
		} else {
			newApp, err := createApp(cmd.Context(), apiClient, organization, appConfig, shared, global)
			if err != nil {
				return err
			}
			appID = newApp
		}

		// Wait for knowledge to be indexed if requested
		if waitKnowledge {
			err = util.WaitForKnowledgeReady(cmd.Context(), apiClient, appID, knowledgeTimeout)
			if err != nil {
				return err
			}
		}

		return nil
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

func createApp(ctx context.Context, apiClient client.Client, orgID string, appConfig *types.AppHelixConfig, shared, global bool) (string, error) {
	org, err := cli.LookupOrganization(ctx, apiClient, orgID)
	if err != nil {
		return "", err
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
		return "", err
	}

	fmt.Printf("%s\n", app.ID)

	return app.ID, nil
}
