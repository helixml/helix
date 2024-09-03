package fs

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:     "remove <file_path>",
	Aliases: []string{"rm"},
	Short:   "Remove a file from the filestore",
	Long:    `Remove a file from the Helix filestore.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("file path is required")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		filePath := args[0]

		// Delete the file
		if err := apiClient.FilestoreDelete(cmd.Context(), filePath); err != nil {
			return fmt.Errorf("failed to delete file: %w", err)
		}

		fmt.Printf("File %s deleted\n", filePath)

		return nil
	},
}
