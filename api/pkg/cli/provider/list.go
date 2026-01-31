package provider

import (
	"fmt"
	"strings"
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
	Short:   "List provider endpoints",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		endpoints, err := apiClient.ListProviderEndpoints(cmd.Context())
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
