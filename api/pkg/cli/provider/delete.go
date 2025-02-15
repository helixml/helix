package provider

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a provider endpoint",
	Long:  ``,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		id := args[0]

		err = apiClient.DeleteProviderEndpoint(cmd.Context(), id)
		if err != nil {
			return fmt.Errorf("failed to delete provider endpoint: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Provider endpoint %s deleted successfully\n", id)
		return nil
	},
}
