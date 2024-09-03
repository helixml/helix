package fs

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
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

		for _, file := range files {
			fmt.Println(file.Name)
		}

		return nil
	},
}
