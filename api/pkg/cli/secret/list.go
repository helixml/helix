package secret

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

var orgFlag string
var projectFlag string

func init() {
	listCmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (env: HELIX_ORG). Lists org-owned secrets instead of personal secrets.")
	listCmd.Flags().StringVarP(&projectFlag, "project", "p", "", "Project ID or name. Lists secrets injected into that project's sessions.")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix secrets in an organization (or for the current user)",
	Long: `List helix secrets.

By default lists the current user's personal secrets. Pass --org to list
secrets owned by an organization, or --project to list project-scoped secrets.
The HELIX_ORG environment variable is also used to resolve a project name.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgRef := orgFlag
		if orgRef == "" {
			orgRef = os.Getenv("HELIX_ORG")
		}
		if projectFlag != "" && cmd.Flags().Changed("org") {
			return fmt.Errorf("--project and --org are mutually exclusive")
		}

		var secrets []*types.Secret
		if projectFlag != "" {
			project, err := cli.LookupProject(cmd.Context(), apiClient, projectFlag, orgRef)
			if err != nil {
				return err
			}
			secrets, err = apiClient.ListProjectSecrets(cmd.Context(), project.ID)
			if err != nil {
				return fmt.Errorf("failed to list project secrets: %w", err)
			}
		} else if orgRef != "" {
			org, err := cli.LookupOrganization(cmd.Context(), apiClient, orgRef)
			if err != nil {
				return err
			}
			secrets, err = apiClient.ListSecrets(cmd.Context(), &client.SecretFilter{OrganizationID: org.ID})
			if err != nil {
				return fmt.Errorf("failed to list secrets: %w", err)
			}
		} else {
			secrets, err = apiClient.ListSecrets(cmd.Context(), nil)
			if err != nil {
				return fmt.Errorf("failed to list secrets: %w", err)
			}
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"Name", "App ID", "Project ID", "Created", "Updated"})

		for _, s := range secrets {
			row := []string{
				s.Name,
				s.AppID,
				s.ProjectID,
				s.Created.Format(time.RFC3339),
				s.Updated.Format(time.RFC3339),
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
