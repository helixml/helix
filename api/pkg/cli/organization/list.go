package organization

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List organizations",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organizations, err := apiClient.ListOrganizations(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to list organizations: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Created"})

		for _, o := range organizations {
			created := o.CreatedAt.Format(time.RFC3339)
			if o.CreatedAt.IsZero() {
				created = "-"
			}

			row := []string{
				o.ID,
				o.Name,
				created,
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
