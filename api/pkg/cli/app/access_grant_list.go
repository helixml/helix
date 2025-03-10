package app

import (
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(listAccessGrantsCmd)
}

var listAccessGrantsCmd = &cobra.Command{
	Use:   "list-access-grants [app ID]",
	Short: "List access grants for an app",
	Long:  `List access grants for an app.`,
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
			return fmt.Errorf("failed to lookup app: %w", err)
		}

		grants, err := apiClient.ListAppAccessGrants(cmd.Context(), &client.AppAccessGrantsFilter{
			AppID: app.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to list access grants: %w", err)
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"ID", "Created", "Team ID", "User"}

		table.SetHeader(header)

		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		table.SetRowSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(false)

		for _, grant := range grants {
			row := []string{
				grant.ID,
				grant.CreatedAt.Format(time.DateTime),
				grant.TeamID,
				grant.User.Email,
			}

			table.Append(row)
		}

		table.Render()
		return nil
	},
}
