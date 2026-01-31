package system

import (
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

var (
	settingsHuggingFaceToken string
	settingsClearHFToken     bool
)

var settingsCmd = &cobra.Command{
	Use:   "settings",
	Short: "Manage system settings",
	Long:  `View and modify global system settings. Requires admin privileges.`,
}

var settingsGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Get current system settings",
	Long:  `Display current system settings including token sources and configuration status.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		settings, err := apiClient.GetSystemSettings(cmd.Context())
		if err != nil {
			return fmt.Errorf("failed to get system settings: %w", err)
		}

		// Display settings in a user-friendly format
		fmt.Println("System Settings:")
		fmt.Printf("  ID: %s\n", settings.ID)
		fmt.Printf("  Created: %s\n", settings.Created.Format("2006-01-02 15:04:05"))
		fmt.Printf("  Updated: %s\n", settings.Updated.Format("2006-01-02 15:04:05"))
		fmt.Println()
		fmt.Println("Hugging Face Token:")
		fmt.Printf("  Configured: %t\n", settings.HuggingFaceTokenSet)
		fmt.Printf("  Source: %s\n", settings.HuggingFaceTokenSource)

		if settings.HuggingFaceTokenSource == "environment" {
			fmt.Println("  Note: Token is read from HF_TOKEN environment variable")
		} else if settings.HuggingFaceTokenSource == "database" {
			fmt.Println("  Note: Token is stored in database (set via API/CLI)")
		} else {
			fmt.Println("  Note: No token configured - only public models will be accessible")
		}

		return nil
	},
}

var settingsSetCmd = &cobra.Command{
	Use:   "set",
	Short: "Set system settings",
	Long:  `Update system settings. Requires admin privileges.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create API client: %w", err)
		}

		// Prepare request payload
		req := types.SystemSettingsRequest{}

		if settingsClearHFToken {
			// Clear the token by setting it to empty string
			emptyToken := ""
			req.HuggingFaceToken = &emptyToken
		} else if settingsHuggingFaceToken != "" {
			req.HuggingFaceToken = &settingsHuggingFaceToken
		}

		// Validate that at least one setting is provided
		if req.HuggingFaceToken == nil {
			return fmt.Errorf("no settings to update. Use --huggingface-token or --clear-hf-token")
		}

		_, err = apiClient.UpdateSystemSettings(cmd.Context(), &req)
		if err != nil {
			return fmt.Errorf("failed to update system settings: %w", err)
		}

		fmt.Println("System settings updated successfully!")

		if settingsClearHFToken {
			fmt.Println("  Hugging Face token: cleared")
			fmt.Println("  Note: Will fall back to HF_TOKEN environment variable if set")
		} else if settingsHuggingFaceToken != "" {
			// Mask the token for security
			maskedToken := maskToken(settingsHuggingFaceToken)
			fmt.Printf("  Hugging Face token: set (%s)\n", maskedToken)
			fmt.Println("  Note: Token will be automatically synced to all connected runners")
		}

		return nil
	},
}

// maskToken masks a token for display purposes
func maskToken(token string) string {
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	return token[:4] + strings.Repeat("*", len(token)-8) + token[len(token)-4:]
}

func init() {
	rootCmd.AddCommand(settingsCmd)
	settingsCmd.AddCommand(settingsGetCmd)
	settingsCmd.AddCommand(settingsSetCmd)

	// Settings set command flags
	settingsSetCmd.Flags().StringVar(&settingsHuggingFaceToken, "huggingface-token", "", "Set Hugging Face token")
	settingsSetCmd.Flags().BoolVar(&settingsClearHFToken, "clear-hf-token", false, "Clear Hugging Face token")
}
