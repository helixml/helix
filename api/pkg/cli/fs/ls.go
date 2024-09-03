package fs

import (
	"time"

	"github.com/dustin/go-humanize"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(lsCmd)
}

var lsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List files in a directory",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var path string

		if len(args) > 0 {
			path = args[0]
		}

		files, err := apiClient.FilestoreList(cmd.Context(), path)
		if err != nil {
			return err
		}

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"Created", "Name", "Size"}

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

		for _, file := range files {
			row := []string{
				time.Unix(file.Created, 0).Format(time.DateTime),
				file.Name,
				humanize.Bytes(uint64(file.Size)),
			}

			table.Append(row)
		}

		table.Render()

		return nil
	},
}
