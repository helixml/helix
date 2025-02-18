package provider

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	createCmd.Flags().StringP("name", "n", "", "Name of the provider endpoint")
	createCmd.Flags().StringP("description", "d", "", "Description of the provider endpoint")
	createCmd.Flags().StringP("type", "t", string(types.ProviderEndpointTypeUser), "Type of the provider endpoint (user, global)")
	createCmd.Flags().StringP("base-url", "u", "", "Base URL of the provider endpoint")
	createCmd.Flags().StringP("api-key", "k", "", "API key for the provider endpoint")
	createCmd.Flags().StringP("api-key-file", "f", "", "Path to file containing API key")
	createCmd.Flags().StringSliceP("models", "m", []string{}, "Comma-separated list of supported models")

	if err := createCmd.MarkFlagRequired("name"); err != nil {
		return
	}
	if err := createCmd.MarkFlagRequired("base-url"); err != nil {
		return
	}

	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new provider endpoint",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		description, _ := cmd.Flags().GetString("description")
		endpointType, _ := cmd.Flags().GetString("type")
		baseURL, _ := cmd.Flags().GetString("base-url")
		apiKey, _ := cmd.Flags().GetString("api-key")
		apiKeyFile, _ := cmd.Flags().GetString("api-key-file")
		models, _ := cmd.Flags().GetStringSlice("models")

		endpoint := &types.ProviderEndpoint{
			Name:           name,
			Description:    description,
			EndpointType:   types.ProviderEndpointType(endpointType),
			BaseURL:        baseURL,
			APIKey:         apiKey,
			APIKeyFromFile: apiKeyFile,
			Models:         models,
		}

		createdEndpoint, err := apiClient.CreateProviderEndpoint(cmd.Context(), endpoint)
		if err != nil {
			return fmt.Errorf("failed to create provider endpoint: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Provider endpoint created successfully:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", createdEndpoint.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", createdEndpoint.Name)
		fmt.Fprintf(cmd.OutOrStdout(), "Type: %s\n", createdEndpoint.EndpointType)
		fmt.Fprintf(cmd.OutOrStdout(), "Base URL: %s\n", createdEndpoint.BaseURL)
		if len(createdEndpoint.Models) > 0 {
			fmt.Fprintf(cmd.OutOrStdout(), "Models: %s\n", strings.Join(createdEndpoint.Models, ", "))
		}

		return nil
	},
}
