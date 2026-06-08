package knowledge

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	listCmd.Flags().StringVarP(&orgFlag, "org", "o", "", "Organization name or ID (defaults to $HELIX_ORG, then your only org, or prompts)")
	rootCmd.AddCommand(listCmd)
}

var orgFlag string

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix knowledge in an organization",
	Long: `List helix knowledge in an organization.

Knowledge is organization-scoped. If --org is omitted and you belong to a
single org, that org is used; if you belong to multiple, you will be
prompted. The HELIX_ORG environment variable is also honoured.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgID, err := cli.ResolveOrganizationInteractive(cmd.Context(), apiClient, orgFlag)
		if err != nil {
			return err
		}

		knowledge, err := apiClient.ListKnowledge(cmd.Context(), &client.KnowledgeFilter{
			OrganizationID: orgID,
		})
		if err != nil {
			return fmt.Errorf("failed to list knowledge: %w", err)
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Name", "Created", "Source", "State", "Refresh", "Schedule", "Next Run", "Version", "Size"})

		for _, k := range knowledge {
			var sourceStr string

			switch {
			case k.Source.Text != nil:
				sourceStr = "plain_content"
			case k.Source.Web != nil:
				sourceStr = "web"
			case k.Source.Filestore != nil:
				sourceStr = "filestore"
			}

			var stateStr string

			if k.State == types.KnowledgeStateError {
				// Truncate the message to 100 characters
				truncatedMessage := k.Message
				if len(truncatedMessage) > 100 {
					truncatedMessage = truncatedMessage[:100] + "..."
				}
				stateStr = fmt.Sprintf("%s (%s)", k.State, truncatedMessage)
			} else {
				stateStr = string(k.State)
			}

			var nextRunStr string

			if k.RefreshEnabled && k.RefreshSchedule != "" && !k.NextRun.IsZero() {
				nextRunStr = k.NextRun.Format(time.RFC3339)
			} else {
				nextRunStr = ""
			}

			row := []string{
				k.ID,
				k.Name,
				k.Created.Format(time.RFC3339),
				sourceStr,
				stateStr,
				strconv.FormatBool(k.RefreshEnabled),
				k.RefreshSchedule,
				nextRunStr,
				k.Version,
				humanize.Bytes(uint64(k.Size)),
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
