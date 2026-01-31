package knowledge

import (
	"fmt"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(versionsListCmd)
}

var versionsListCmd = &cobra.Command{
	Use:     "versions",
	Aliases: []string{"v"},
	Short:   "List versions of a specific knowledge",
	Long:    ``,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		knowledge, err := lookupKnowledge(cmd.Context(), apiClient, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup knowledge: %w", err)
		}

		versions, err := apiClient.ListKnowledgeVersions(cmd.Context(), &client.KnowledgeVersionsFilter{
			KnowledgeID: knowledge.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to list knowledge versions: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Created", "State", "Message", "Version", "Size"})

		for _, v := range versions {
			row := []string{
				v.ID,
				v.Created.Format(time.RFC3339),
				string(v.State),
				v.Message,
				v.Version,
				humanize.Bytes(uint64(v.Size)),
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
