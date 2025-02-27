package app

import (
	"fmt"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(listCmd)

	// listCmd.Flags().StringP("organization", "o", "", "Organization ID or name")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// listCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// listCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix apps",
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
			return fmt.Errorf("failed to list apps: %w", err)
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"ID", "Name", "Created", "Source"}

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

		for _, app := range apps {
			row := []string{
				app.ID,
				app.Config.Helix.Name,
				app.Created.Format(time.DateTime),
				string(app.AppSource),
			}

			table.Append(row)
		}

		table.Render()

		return nil
	},
}
