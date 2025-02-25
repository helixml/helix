package roles

import (
	"fmt"
	"strings"
	"time"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
)

func init() {
	listCmd.Flags().StringP("organization", "o", "", "Organization ID")

	if err := listCmd.MarkFlagRequired("organization"); err != nil {
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

		roles, err := apiClient.ListOrganizationRoles(cmd.Context(), reference)
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
