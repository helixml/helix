package roles

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
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

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Description"})

		for _, r := range roles {
			row := []string{
				r.ID,
				r.Name,
				r.Description,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
