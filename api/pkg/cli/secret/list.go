package secret

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

var orgFlag string

func init() {
	listCmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (env: HELIX_ORG). Lists org-owned secrets instead of personal secrets.")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix secrets in an organization (or for the current user)",
	Long: `List helix secrets.

By default lists the current user's personal secrets. Pass --org to list
secrets owned by an organization instead. The HELIX_ORG environment
variable is honoured if --org is not given but you want org-scoped results.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var filter *client.SecretFilter
		orgRef := orgFlag
		if orgRef == "" {
			orgRef = os.Getenv("HELIX_ORG")
		}
		if orgRef != "" {
			org, err := cli.LookupOrganization(cmd.Context(), apiClient, orgRef)
			if err != nil {
				return err
			}
			filter = &client.SecretFilter{OrganizationID: org.ID}
		}

		secrets, err := apiClient.ListSecrets(cmd.Context(), filter)
		if err != nil {
			return fmt.Errorf("failed to list secrets: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"Name", "App ID", "Created", "Updated"})

		for _, s := range secrets {
			row := []string{
				s.Name,
				s.AppID,
				s.Created.Format(time.RFC3339),
				s.Updated.Format(time.RFC3339),
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
