package moonlight

import (
	"encoding/json"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var pairCmd = &cobra.Command{
	Use:   "pair [pair-secret] [pin]",
	Short: "Complete Moonlight client pairing",
	Long: `Complete the pairing process for a Moonlight client using the pair secret and PIN.

The pair secret can be obtained from 'helix moonlight list-pending' and the PIN
is displayed on the Moonlight client during the pairing process.`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		pairSecret := args[0]
		pin := args[1]

		// Create request payload
		payload := map[string]interface{}{
			"pair_secret": pairSecret,
			"pin":         pin,
		}

		payloadBytes, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("failed to marshal request: %w", err)
		}

		fmt.Printf("Pairing Moonlight client with secret '%s' and PIN '%s'...\n", pairSecret, pin)

		resp, err := apiClient.CompleteWolfPairing(cmd.Context(), payloadBytes)
		if err != nil {
			return fmt.Errorf("failed to complete pairing: %w", err)
		}

		// Parse response
		var response map[string]interface{}
		if err := json.Unmarshal(resp, &response); err != nil {
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