package provider

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:     "delete [id]",
	Short:   "Delete a provider endpoint",
	Aliases: []string{"rm"},
	Long:    ``,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		id := args[0]

		// If provider ID does not start with "pe_" then look up by name
		if !strings.HasPrefix(id, "pe_") {
			providers, err := apiClient.ListProviderEndpoints(cmd.Context())
			if err != nil {
				return fmt.Errorf("failed to get provider endpoint: %w", err)
			}

			for _, provider := range providers {
				if provider.Name == id {
					id = provider.ID
					break
				}
			}
		}

		err = apiClient.DeleteProviderEndpoint(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to delete provider endpoint: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Provider endpoint %s deleted successfully\n", id)
		return nil
	},
}
