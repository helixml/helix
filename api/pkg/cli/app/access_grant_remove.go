package app

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeAccessCmd)
}

var removeAccessCmd = &cobra.Command{
	Use:   "remove-access [agent ID] [access grant ID]",
	Short: "Remove access from a user or team",
	Long:  `Remove access from a user or team.`,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		app, err := lookupApp(cmd.Context(), apiClient, organization, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup agent: %w", err)
		}

		grants, err := apiClient.ListAppAccessGrants(cmd.Context(), &client.AppAccessGrantsFilter{
			AppID: app.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to list access grants: %w", err)
		}

		for _, grant := range grants {
			if grant.ID == args[1] {
				err := apiClient.DeleteAppAccessGrant(cmd.Context(), app.ID, grant.ID)
				if err != nil {
					return fmt.Errorf("failed to delete access grant: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Access grant '%s' deleted successfully\n", grant.ID)
				return nil
			}
		}
		return fmt.Errorf("access grant not found")
	},
}
