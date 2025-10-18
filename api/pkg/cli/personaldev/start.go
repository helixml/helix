package personaldev

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [environment-id]",
	Short: "Start a personal development environment",
	Long:  `Start a personal development environment by its ID`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		environmentID := args[0]

		fmt.Printf("Starting personal dev environment '%s'...\n", environmentID)

		err = apiClient.StartPersonalDevEnvironment(cmd.Context(), environmentID)
		if err != nil {
			return fmt.Errorf("failed to start personal dev environment: %w", err)
		}

		fmt.Printf("✅ Personal dev environment '%s' started successfully!\n", environmentID)
		return nil
	},
}