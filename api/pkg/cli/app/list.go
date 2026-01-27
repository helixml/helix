package app

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix agents",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		filter := &client.AppFilter{}

		if organization != "" {
			org, err := cli.LookupOrganization(cmd.Context(), apiClient, organization)
			if err != nil {
				return err
			}

			filter.OrganizationID = org.ID
		}

		apps, err := apiClient.ListApps(cmd.Context(), filter)
		if err != nil {
			return fmt.Errorf("failed to list agents: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Created", "Owner"})

		for _, app := range apps {
			row := []string{
				app.ID,
				app.Config.Helix.Name,
				app.Created.Format(time.DateTime),
				app.User.Email,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
