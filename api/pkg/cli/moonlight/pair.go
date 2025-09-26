package moonlight

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var pairCmd = &cobra.Command{
	Use:   "pair [pin]",
	Short: "Complete Moonlight client pairing",
	Long: `Complete the pairing process for a Moonlight client using the PIN.

The PIN is displayed on the Moonlight client during the pairing process.
This command will automatically find and pair with pending pairing requests.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		pin := args[0]

		// First, get pending pairing requests
		fmt.Println("Getting pending pairing requests...")
		resp, err := apiClient.GetWolfPendingPairRequests(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to get pending pair requests: %w", err)
		}

		// Parse pending requests
		var requests []map[string]interface{}
		if err := json.Unmarshal(resp, &requests); err != nil {
			return fmt.Errorf("failed to parse pending requests: %w", err)
		}

		if len(requests) == 0 {
			return fmt.Errorf("no pending pairing requests found. Please start the pairing process on your Moonlight client first")
		}

		// If multiple requests, use the first one (or we could ask user to choose)
		pairSecret := getStringField(requests[0], "pair_secret")
		if pairSecret == "" {
			return fmt.Errorf("invalid pairing request: missing pair_secret")
		}

		// Create request payload
		payload := map[string]interface{}{
			"pair_secret": pairSecret,
			"pin":         pin,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		fmt.Printf("Pairing Moonlight client with PIN '%s'...\n", pin)

		pairResp, err := apiClient.CompleteWolfPairing(cmd.Context(), payloadBytes)
		if err != nil {
			return fmt.Errorf("failed to complete pairing: %w", err)
		}

		// Parse response
		var response map[string]interface{}
		if err := json.Unmarshal(pairResp, &response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		if success, ok := response["success"].(bool); ok && success {
			fmt.Println("✅ Moonlight client paired successfully!")
			if message, ok := response["message"].(string); ok && message != "" {
				fmt.Printf("Message: %s\n", message)
			}
		} else {
			return fmt.Errorf("pairing failed: %v", response)
		}

		fmt.Println("\nYour Moonlight client should now be able to connect and stream applications.")
		fmt.Println("You can create personal dev environments using: helix personaldev create <name> --app-id <app-id>")

		return nil
	},
}