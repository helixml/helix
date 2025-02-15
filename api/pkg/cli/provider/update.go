package provider

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	updateCmd.Flags().StringP("name", "n", "", "Name of the provider endpoint")
	updateCmd.Flags().StringP("description", "d", "", "Description of the provider endpoint")
	updateCmd.Flags().StringP("type", "t", "", "Type of the provider endpoint (user, global)")
	updateCmd.Flags().StringP("base-url", "u", "", "Base URL of the provider endpoint")
	updateCmd.Flags().StringP("api-key", "k", "", "API key for the provider endpoint")
	updateCmd.Flags().StringP("api-key-file", "f", "", "Path to file containing API key")
	updateCmd.Flags().StringSliceP("models", "m", []string{}, "Comma-separated list of supported models")

	rootCmd.AddCommand(updateCmd)
}

var updateCmd = &cobra.Command{
	Use:   "update [id]",
	Short: "Update a provider endpoint",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		id := args[0]

		// First get the existing endpoint
		existingEndpoint, err := apiClient.GetProviderEndpoint(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to get provider endpoint: %w", err)
		}

		// Update fields if provided in flags
		if name, _ := cmd.Flags().GetString("name"); name != "" {
			existingEndpoint.Name = name
		}
		if description, _ := cmd.Flags().GetString("description"); description != "" {
			existingEndpoint.Description = description
		}
		if endpointType, _ := cmd.Flags().GetString("type"); endpointType != "" {
			existingEndpoint.EndpointType = types.ProviderEndpointType(endpointType)
		}
		if baseURL, _ := cmd.Flags().GetString("base-url"); baseURL != "" {
			existingEndpoint.BaseURL = baseURL
		}
		if apiKey, _ := cmd.Flags().GetString("api-key"); apiKey != "" {
			existingEndpoint.APIKey = apiKey
		}
		if apiKeyFile, _ := cmd.Flags().GetString("api-key-file"); apiKeyFile != "" {
			existingEndpoint.APIKeyFromFile = apiKeyFile
		}
		if models, _ := cmd.Flags().GetStringSlice("models"); len(models) > 0 {
			existingEndpoint.Models = models
		}

		updatedEndpoint, err := apiClient.UpdateProviderEndpoint(cmd.Context(), existingEndpoint)
		if err != nil {
			return fmt.Errorf("failed to update provider endpoint: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Provider endpoint updated successfully:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", updatedEndpoint.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", updatedEndpoint.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Type: %s\n", updatedEndpoint.EndpointType)
		fmt.Fprintf(cmd.OutOrStdout(), "Base URL: %s\n", updatedEndpoint.BaseURL)
		if len(updatedEndpoint.Models) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Models: %s\n", strings.Join(updatedEndpoint.Models, ", "))
		}

		return nil
	},
}
