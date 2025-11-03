package personaldev

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [environment-id]",
	Short: "Stop a personal development environment",
	Long:  `Stop a personal development environment by its ID`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		environmentID := args[0]

		fmt.Printf("Stopping personal dev environment '%s'...\n", environmentID)

		err = apiClient.StopPersonalDevEnvironment(cmd.Context(), environmentID)
		if err != nil {
			return fmt.Errorf("failed to stop personal dev environment: %w", err)
		}

		fmt.Printf("✅ Personal dev environment '%s' stopped successfully!\n", environmentID)
		return nil
	},
}