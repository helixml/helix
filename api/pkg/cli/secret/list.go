package secret

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
	Short:   "List helix secrets",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		secrets, err := apiClient.ListSecrets(cmd.Context())
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
