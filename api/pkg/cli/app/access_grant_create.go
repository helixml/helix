package app

import (
	"fmt"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(grantAccessCmd)

	grantAccessCmd.Flags().StringP("user", "u", "", "User ID or email to grant access to")
	grantAccessCmd.Flags().StringP("team", "t", "", "Team ID to grant access to")
	grantAccessCmd.Flags().StringSliceP("roles", "r", []string{}, "Roles to grant to the user")
}

var grantAccessCmd = &cobra.Command{
	Use:   "grant-access [agent ID] --user [user ID] --team [team ID] --roles [roles]",
	Short: "Grant access to a user or team",
	Long:  `Grant access to a user or team.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		organization, err := cmd.Flags().GetString("organization")
		if err != nil {
			return err
		}

		app, err := lookupApp(cmd.Context(), apiClient, organization, args[0])
		if err != nil {
			return fmt.Errorf("failed to lookup agent: %w", err)
		}

		if app.OrganizationID == "" {
			return fmt.Errorf("agent is not associated with an organization")
		}

		user, err := cmd.Flags().GetString("user")
		if err != nil {
			return err
		}

		teamRef, err := cmd.Flags().GetString("team")
		if err != nil {
			return err
		}

		roles, err := cmd.Flags().GetStringSlice("roles")
		if err != nil {
			return err
		}

		if len(roles) == 0 {
			return fmt.Errorf("specify at least one role, available roles: %v", getRoleNames())
		}

		team, err := cli.LookupTeam(cmd.Context(), apiClient, app.OrganizationID, teamRef)
		if err != nil {
			return fmt.Errorf("failed to lookup team: %w", err)
		}

		accessGrant := &types.CreateAccessGrantRequest{
			UserReference: user,
			TeamID:        team.ID,
			Roles:         roles,
		}

		created, err := apiClient.CreateAppAccessGrant(cmd.Context(), app.ID, accessGrant)
		if err != nil {
			return fmt.Errorf("failed to create access grant: %w", err)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Access grant '%s' created successfully\n", created.ID)
		return nil
	},
}

func getRoleNames() []string {
	var names []string
	for _, role := range types.Roles {
		names = append(names, role.Name)
	}
	return names
}
