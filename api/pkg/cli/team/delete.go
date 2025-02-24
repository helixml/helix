package team

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	deleteCmd.Flags().StringP("organization", "o", "", "Organization ID or name")
	deleteCmd.Flags().StringP("team", "t", "", "Team ID")

	if err := deleteCmd.MarkFlagRequired("organization"); err != nil {
		return
	}
	if err := deleteCmd.MarkFlagRequired("team"); err != nil {
		return
	}

	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:     "delete",
	Short:   "Delete a team",
	Aliases: []string{"rm"},
	Long:    `Delete a team from an organization. You must be an organization owner to delete teams.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgRef, _ := cmd.Flags().GetString("organization")
		teamRef, _ := cmd.Flags().GetString("team")

		org, err := lookupOrganization(cmd.Context(), apiClient, orgRef)
		if err != nil {
			return fmt.Errorf("failed to lookup organization: %w", err)
		}

		team, err := lookupTeam(cmd.Context(), apiClient, org.ID, teamRef)
		if err != nil {
			return fmt.Errorf("failed to lookup team: %w", err)
		}

		err = apiClient.DeleteTeam(cmd.Context(), org.ID, team.ID)
		if err != nil {
			return fmt.Errorf("failed to delete team: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Team %s deleted successfully\n", team.ID)
		return nil
	},
}
