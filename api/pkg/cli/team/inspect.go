package team

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
)

func init() {
	inspectCmd.Flags().StringP("organization", "o", "", "Organization ID or name")
	inspectCmd.Flags().StringP("team", "t", "", "Team name or ID")

	if err := inspectCmd.MarkFlagRequired("organization"); err != nil {
		return
	}
	// if err := inspectCmd.MarkFlagRequired("team"); err != nil {
	// 	return
	// }

	rootCmd.AddCommand(inspectCmd)
}

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Inspect a team",
	Long:  `Inspect a team in an organization.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgRef, _ := cmd.Flags().GetString("organization")

		// Either team flag is set or the first argument is a team name
		teamRef, _ := cmd.Flags().GetString("team")
		if teamRef == "" {
			if len(args) == 0 {
				return fmt.Errorf("team name or ID is required")
			}
			teamRef = args[0]
		}

		org, err := lookupOrganization(cmd.Context(), apiClient, orgRef)
		if err != nil {
			return fmt.Errorf("failed to lookup organization: %w", err)
		}

		team, err := lookupTeam(cmd.Context(), apiClient, org.ID, teamRef)
		if err != nil {
			return fmt.Errorf("failed to lookup team: %w", err)
		}

		// Pretty print JSON
		json, err := json.MarshalIndent(team, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal team: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\n", string(json))

		return nil
	},
}
