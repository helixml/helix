package provider

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

var orgFlag string

func init() {
	listCmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (env: HELIX_ORG). Lists org-owned provider endpoints instead of personal ones.")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List provider endpoints (personal, or for an organization with --org)",
	Long: `List provider endpoints.

By default lists the current user's personal provider endpoints plus any
global ones. Pass --org to list provider endpoints owned by an
organization. The HELIX_ORG environment variable is honoured.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var filter *client.ProviderEndpointFilter
		orgRef := orgFlag
		if orgRef == "" {
			orgRef = os.Getenv("HELIX_ORG")
		}
		if orgRef != "" {
			org, err := cli.LookupOrganization(cmd.Context(), apiClient, orgRef)
			if err != nil {
				return err
			}
			filter = &client.ProviderEndpointFilter{OrganizationID: org.ID}
		}

		endpoints, err := apiClient.ListProviderEndpoints(cmd.Context(), filter)
		if err != nil {
			return fmt.Errorf("failed to list provider endpoints: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Type", "Base URL", "Models", "Created"})

		for _, e := range endpoints {
			created := e.Created.Format(time.RFC3339)
			if e.Created.IsZero() {
				created = "-"
			}

			row := []string{
				e.ID,
				e.Name,
				string(e.EndpointType),
				e.BaseURL,
				strings.Join(e.Models, ", "),
				created,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
