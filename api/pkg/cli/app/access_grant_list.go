package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(listAccessGrantsCmd)
}

var listAccessGrantsCmd = &cobra.Command{
	Use:   "list-access-grants [agent ID]",
	Short: "List access grants for an agent",
	Long:  `List access grants for an agent.`,
	Args:  cobra.ExactArgs(1),
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

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Created", "Team ID", "User"})

		for _, grant := range grants {
			row := []string{
				grant.ID,
				grant.CreatedAt.Format(time.DateTime),
				grant.TeamID,
				grant.User.Email,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)
		return nil
	},
}
