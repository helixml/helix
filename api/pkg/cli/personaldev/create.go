package personaldev

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var createCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new personal development environment",
	Long:  `Create a new personal development environment with the specified name`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		environmentName := args[0]
		appID, _ := cmd.Flags().GetString("app-id")
		description, _ := cmd.Flags().GetString("description")

		// Validate app-id is provided
		if appID == "" {
			return fmt.Errorf("--app-id is required")
		}

		// Create request payload
		payload := map[string]interface{}{
			"environment_name": environmentName,
			"app_id":          appID,
		}
		if description != "" {
			payload["description"] = description
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		fmt.Printf("Creating personal dev environment '%s' with app '%s'...\n", environmentName, appID)

		resp, err := apiClient.CreatePersonalDevEnvironment(cmd.Context(), payloadBytes)
		if err != nil {
			return fmt.Errorf("failed to create personal dev environment: %w", err)
		}

		// Parse response
		var environment map[string]interface{}
		if err := json.Unmarshal(resp, &environment); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		fmt.Printf("✅ Personal dev environment created successfully!\n")
		fmt.Printf("ID: %s\n", getStringField(environment, "instance_id"))
		fmt.Printf("Name: %s\n", getStringField(environment, "environment_name"))
		fmt.Printf("Status: %s\n", getStringField(environment, "status"))

		if streamURL := getStringField(environment, "stream_url"); streamURL != "" {
			fmt.Printf("Stream URL: %s\n", streamURL)
		}

		return nil
	},
}

func init() {
	createCmd.Flags().StringP("app-id", "a", "", "Helix agent ID to use for the environment (required)")
	createCmd.Flags().StringP("description", "d", "", "Description for the environment")
	createCmd.MarkFlagRequired("app-id")
}

