package app

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(removeCmd)

	removeCmd.Flags().Bool("knowledge", true, "Delete knowledge")
}

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Delete helix app",
	Long:    ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("app name or ID is required")
		}

		knowledge, err := cmd.Flags().GetBool("knowledge")
		if err != nil {
			return err
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		app, err := lookupApp(cmd.Context(), apiClient, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup app: %w", err)
		}

		// Delete the app
		if err := apiClient.DeleteApp(cmd.Context(), app.ID, knowledge); err != nil {
			return fmt.Errorf("failed to delete app: %w", err)
		}

		fmt.Printf("App %s deleted\n", app.ID)

		return nil
	},
}
