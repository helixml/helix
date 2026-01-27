package fs

import (
	"time"

	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
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

		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"Created", "Name", "Size"})

		for _, file := range files {
			row := []string{
				time.Unix(file.Created, 0).Format(time.DateTime),
				file.Name,
				humanize.Bytes(uint64(file.Size)),
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}
