package roles

import (
	"fmt"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	listCmd.Flags().StringP("organization-id", "o", "", "Organization ID")

	if err := listCmd.MarkFlagRequired("organization-id"); err != nil {
		return
	}

	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List roles for the given organization",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organizationID, _ := cmd.Flags().GetString("organization-id")

		roles, err := apiClient.ListOrganizationRoles(cmd.Context(), organizationID)
		if err != nil {
			return fmt.Errorf("failed to list roles: %w", err)
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"ID", "Name", "Description", "Created"}

		table.SetHeader(header)

		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		// table.SetRowSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(false)

		for _, r := range roles {
			created := r.CreatedAt.Format(time.RFC3339)
			if r.CreatedAt.IsZero() {
				created = "-"
			}

			row := []string{
				r.ID,
				r.Name,
				r.Description,
				created,
			}

			table.Append(row)
		}

		table.Render()

		return nil
	},
}
