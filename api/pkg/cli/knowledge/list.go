package knowledge

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dustin/go-humanize"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List helix knowledge",
	Long:    ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		knowledge, err := apiClient.ListKnowledge(&client.KnowledgeFilter{})
		if err != nil {
			return fmt.Errorf("failed to list knowledge: %w", err)
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"ID", "Name", "Created", "Source", "State", "Refresh", "Schedule", "Version", "Size"}

		table.SetHeader(header)

		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		table.SetRowSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(false)

		for _, k := range knowledge {
			var sourceStr string

			switch {
			case k.Source.Content != nil:
				sourceStr = "plain_content"
			case k.Source.Web != nil:
				sourceStr = "web"
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

			row := []string{
				k.ID,
				k.Name,
				k.Created.Format(time.RFC3339),
				sourceStr,
				stateStr,
				strconv.FormatBool(k.RefreshEnabled),
				k.RefreshSchedule,
				k.Version,
				humanize.Bytes(uint64(k.Size)),
			}

			table.Append(row)
		}

		table.Render()

		return nil
	},
}
