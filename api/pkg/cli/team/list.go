package team

import (
	"fmt"
	"strings"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
)

func init() {
	listCmd.Flags().StringP("organization", "o", "", "Organization ID or name")

	if err := listCmd.MarkFlagRequired("organization"); err != nil {
		return
	}

	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List teams in an organization",
	Long:    `List all teams in an organization. You can specify the organization by ID or name.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		reference, _ := cmd.Flags().GetString("organization")
		// If the reference doesn't start with org_ prefix, assume it's a name
		if !strings.HasPrefix(reference, system.OrganizationPrefix) {
			// List organizations to find the one with matching name
			organizations, err := apiClient.ListOrganizations(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to list organizations: %w", err)
			}

			found := false
			for _, org := range organizations {
				if org.Name == reference {
					reference = org.ID
					found = true
					break
				}
			}

			if !found {
				return fmt.Errorf("no organization found with name: %s", reference)
			}
		}

		teams, err := apiClient.ListTeams(cmd.Context(), reference)
		if err != nil {
			return fmt.Errorf("failed to list teams: %w", err)
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())
		table.SetHeader([]string{"ID", "Name", "Created"})

		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(false)

		for _, team := range teams {
			table.Append([]string{
				team.ID,
				team.Name,
				team.CreatedAt.Format("2006-01-02 15:04:05"),
			})
		}

		table.Render()
		return nil
	},
}
