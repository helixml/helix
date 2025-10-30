package personaldev

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [environment-id]",
	Short: "Delete a personal development environment",
	Long:  `Delete a personal development environment by its ID`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		environmentID := args[0]
		force, _ := cmd.Flags().GetBool("force")

		if !force {
			fmt.Printf("Are you sure you want to delete environment '%s'? (y/N): ", environmentID)
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" {
				fmt.Println("Deletion cancelled.")
				return nil
			}
		}

		fmt.Printf("Deleting personal dev environment '%s'...\n", environmentID)

		err = apiClient.DeletePersonalDevEnvironment(cmd.Context(), environmentID)
		if err != nil {
			return fmt.Errorf("failed to delete personal dev environment: %w", err)
		}

		fmt.Printf("✅ Personal dev environment '%s' deleted successfully!\n", environmentID)
		return nil
	},
}

func init() {
	deleteCmd.Flags().BoolP("force", "f", false, "Force deletion without confirmation")
}