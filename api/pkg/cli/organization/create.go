package organization

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	createCmd.Flags().StringP("name", "n", "", "Name of the organization")
	createCmd.Flags().StringP("display-name", "d", "", "Display name of the organization")

	if err := createCmd.MarkFlagRequired("name"); err != nil {
		return
	}

	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new organization",
	Long:  ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		name, _ := cmd.Flags().GetString("name")
		displayName, _ := cmd.Flags().GetString("display-name")

		organization := &types.Organization{
			Name:        name,
			DisplayName: displayName,
		}

		createdOrg, err := apiClient.CreateOrganization(cmd.Context(), organization)
		if err != nil {
			return fmt.Errorf("failed to create organization: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Organization created successfully:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", createdOrg.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", createdOrg.Name)
		if createdOrg.DisplayName != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "Display Name: %s\n", createdOrg.DisplayName)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Owner: %s\n", createdOrg.Owner)

		return nil
	},
}
