package organization

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/system"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id-or-name]",
	Short: "Delete an organization",
	Long:  `Delete an organization by its ID or name. If a name is provided, it will first list organizations to find the matching ID.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		reference := args[0]

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

		err = apiClient.DeleteOrganization(cmd.Context(), reference)
		if err != nil {
			return fmt.Errorf("failed to delete organization: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Organization deleted successfully\n")
		return nil
	},
}
