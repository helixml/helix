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

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(applyCmd)

	applyCmd.Flags().StringP("filename", "f", "", "Filename to apply")
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
	Short: "Apply a Helix agent configuration",
	RunE: func(cmd *cobra.Command, _ []string) error {
		// Get flags
		yamlFile, err := cmd.Flags().GetString("filename")
		if err != nil {
			return err
		}
		if yamlFile == "" {
			return fmt.Errorf("filename is required")
		}

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		global, err := cmd.Flags().GetBool("global")
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

		refreshKnowledge, err := cmd.Flags().GetBool("refresh-knowledge")
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

		// Read and parse the YAML file
		appConfig, err := apps.NewLocalApp(yamlFile)
		if err != nil {
			return err
		}

		// Create API client
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		// Handle app creation or update
		existingApps, err := apiClient.ListApps(cmd.Context(), &client.AppFilter{})
		if err != nil {
			return err
		}

		var existingApp *types.App
		var appID string

		for _, app := range existingApps {
			if app.Config.Helix.Name == appConfig.GetAppConfig().Name {
				existingApp = app
				appID = app.ID
				break
			}
		}

		if existingApp != nil {
			err = updateApp(cmd.Context(), apiClient, existingApp, appConfig.GetAppConfig(), global)
			if err != nil {
				return err
			}
		} else {
			appID, err = createApp(cmd.Context(), apiClient, organization, appConfig.GetAppConfig(), global)
			if err != nil {
				return err
			}
		}

		// Handle the --sync flag to sync local files to filestore
		if len(syncFiles) > 0 {
			mappings, err := util.ParseSyncMappings(syncFiles, appConfig.GetAppConfig())
			if err != nil {
				return err
			}

			for _, mapping := range mappings {
				fmt.Printf("Syncing local directory '%s' to knowledge source '%s' (path: %s)\n",
					mapping.LocalDir, mapping.KnowledgeName, mapping.RemotePath)

				_, err = util.SyncLocalDirToFilestore(cmd.Context(), apiClient, mapping.LocalDir, mapping.RemotePath, deleteExtraFiles, appID)
				if err != nil {
					return fmt.Errorf("failed to sync files for knowledge '%s': %w", mapping.KnowledgeName, err)
				}
			}

			// After syncing files, refresh the knowledge to reindex
			err = RefreshAppKnowledge(cmd.Context(), apiClient, appID)
			if err != nil {
				return err
			}

			// Since we've already reindexed after syncing, skip the explicit refresh
			refreshKnowledge = false
		}

		// Handle explicit refresh-knowledge flag
		if refreshKnowledge {
			err = RefreshAppKnowledge(cmd.Context(), apiClient, appID)
			if err != nil {
				return err
			}
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

// RefreshAppKnowledge triggers a reindex of all knowledge sources for an app
func RefreshAppKnowledge(ctx context.Context, apiClient client.Client, appID string) error {
	knowledgeFilter := &client.KnowledgeFilter{
		AppID: appID,
	}

	knowledge, err := apiClient.ListKnowledge(ctx, knowledgeFilter)
	if err != nil {
		return err
	}

	for _, knowledge := range knowledge {
		err = apiClient.RefreshKnowledge(ctx, knowledge.ID)
		if err != nil {
			return fmt.Errorf("failed to refresh knowledge %s (%s): %w", knowledge.ID, knowledge.Name, err)
		}
	}
	return nil
}

func updateApp(ctx context.Context, apiClient client.Client, app *types.App, appConfig *types.AppHelixConfig, global bool) error {
	app.Config.Helix = *appConfig
	app.Global = global

	app, err := apiClient.UpdateApp(ctx, app)
	if err != nil {
		return err
	}

	fmt.Printf("%s\n", app.ID)

	return nil
}

func createApp(ctx context.Context, apiClient client.Client, orgID string, appConfig *types.AppHelixConfig, global bool) (string, error) {
	app := &types.App{
		Global: global,
		Config: types.AppConfig{
			AllowedDomains: []string{}, // TODO: make configurable
			Helix:          *appConfig,
		},
	}

	// Only set OrganizationID if an organization is provided
	if orgID != "" {
		org, err := cli.LookupOrganization(ctx, apiClient, orgID)
		if err != nil {
			return "", err
		}
		app.OrganizationID = org.ID
	}

	app, err := apiClient.CreateApp(ctx, app)
	if err != nil {
		return "", err
	}

	fmt.Printf("%s\n", app.ID)

	return app.ID, nil
}
