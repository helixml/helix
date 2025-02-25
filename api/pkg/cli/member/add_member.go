package member

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

func init() {
	addCmd.Flags().StringP("organization", "o", "", "Organization ID or name")
	addCmd.Flags().StringP("team", "t", "", "Team ID or name")
	addCmd.Flags().StringP("user-email", "u", "", "User email")

	if err := addCmd.MarkFlagRequired("organization"); err != nil {
		return
	}
	// if err := addCmd.MarkFlagRequired("user-email"); err != nil {
	// 	return
	// }

	rootCmd.AddCommand(addCmd)
}

var addCmd = &cobra.Command{
	Use:   "add",
	Short: "Add a member to an organization or team. If team is provided, the user will be added to the team. For example: `helix member add --organization my-org --team my-team --user-email user@example.com`",
	Long:  ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgReference, _ := cmd.Flags().GetString("organization")
		teamReference, _ := cmd.Flags().GetString("team")
		userEmail, _ := cmd.Flags().GetString("user-email")

		if userEmail == "" && len(args) == 0 {
			return fmt.Errorf("--user-email or and argument is required")
		}

		if userEmail != "" && len(args) > 0 {
			return fmt.Errorf("--user-email and argument cannot both be provided")
		}

		if userEmail == "" {
			userEmail = args[0]
		}

		if teamReference == "" {
			err = addOrgMember(cmd.Context(), apiClient, orgReference, userEmail)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "User '%s' added to organization %s\n", userEmail, orgReference)
			return nil
		}

		err = addTeamMember(cmd.Context(), apiClient, orgReference, teamReference, userEmail)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "User '%s' added to team %s\n", userEmail, teamReference)
		return nil
	},
}

func addOrgMember(ctx context.Context, apiClient *client.HelixClient, orgReference, userReference string) error {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return fmt.Errorf("failed to lookup organization: %w", err)
	}

	_, err = apiClient.AddOrganizationMember(ctx, org.ID, &types.AddOrganizationMemberRequest{
		UserReference: userReference,
	})
	if err != nil {
		return fmt.Errorf("failed to add organization member: %w", err)
	}

	return nil
}

func addTeamMember(ctx context.Context, apiClient *client.HelixClient, orgReference, teamReference, userEmail string) error {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return fmt.Errorf("failed to lookup organization: %w", err)
	}

	team, err := lookupTeam(ctx, apiClient, org.ID, teamReference)
	if err != nil {
		return fmt.Errorf("failed to lookup team: %w", err)
	}

	_, err = apiClient.AddTeamMember(ctx, org.ID, team.ID, &types.AddTeamMemberRequest{
		UserReference: userEmail,
	})
	if err != nil {
		return fmt.Errorf("failed to add team member: %w", err)
	}

	return nil
}
