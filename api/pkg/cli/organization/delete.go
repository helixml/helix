package organization

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:     "delete [id-or-name]",
	Short:   "Delete an organization",
	Aliases: []string{"remove", "rm"},
	Long:    `Delete an organization by its ID or name. If a name is provided, it will first list organizations to find the matching ID.`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		reference := args[0]

		org, err := cli.LookupOrganization(cmd.Context(), apiClient, reference)
		if err != nil {
			return fmt.Errorf("failed to lookup organization: %w", err)
		}

		err = apiClient.DeleteOrganization(cmd.Context(), org.ID)
		if err != nil {
			return fmt.Errorf("failed to delete organization: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Organization deleted successfully\n")
		return nil
	},
}
