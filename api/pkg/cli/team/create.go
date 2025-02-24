package team

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	createCmd.Flags().StringP("organization", "o", "", "Organization ID or name")
	createCmd.Flags().StringP("name", "n", "", "Team name")

	if err := createCmd.MarkFlagRequired("organization"); err != nil {
		return
	}
	if err := createCmd.MarkFlagRequired("name"); err != nil {
		return
	}

	rootCmd.AddCommand(createCmd)
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new team",
	Long:  `Create a new team in an organization. You must be an organization owner to create teams.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgRef, _ := cmd.Flags().GetString("organization")
		org, err := lookupOrganization(cmd.Context(), apiClient, orgRef)
		if err != nil {
			return fmt.Errorf("failed to lookup organization: %w", err)
		}

		name, _ := cmd.Flags().GetString("name")
		team, err := apiClient.CreateTeam(cmd.Context(), org.ID, &types.CreateTeamRequest{
			Name:           name,
			OrganizationID: org.ID,
		})
		if err != nil {
			return fmt.Errorf("failed to create team: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Team created successfully:\n")
		fmt.Fprintf(cmd.OutOrStdout(), "ID: %s\n", team.ID)
		fmt.Fprintf(cmd.OutOrStdout(), "Name: %s\n", team.Name)

		return nil
	},
}
