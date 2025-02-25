package member

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	removeCmd.Flags().StringP("organization", "o", "", "Organization ID or name")
	removeCmd.Flags().StringP("team", "t", "", "Team ID or name")
	removeCmd.Flags().StringP("user-email", "u", "", "User email")
	removeCmd.Flags().BoolP("force", "f", false, "Skip confirmation prompt")

	if err := removeCmd.MarkFlagRequired("organization"); err != nil {
		return
	}

	rootCmd.AddCommand(removeCmd)
}

var removeCmd = &cobra.Command{
	Use:     "remove",
	Aliases: []string{"rm"},
	Short:   "Remove a member from an organization or team. If team is provided, the user will be removed only from the team. For example: `helix member remove --organization my-org --team my-team --user-email user@example.com`",
	Long:    ``,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgReference, _ := cmd.Flags().GetString("organization")
		teamReference, _ := cmd.Flags().GetString("team")
		userEmail, _ := cmd.Flags().GetString("user-email")
		force, _ := cmd.Flags().GetBool("force")

		if userEmail == "" && len(args) == 0 {
			return fmt.Errorf("--user-email or an argument is required")
		}

		if userEmail != "" && len(args) > 0 {
			return fmt.Errorf("--user-email and argument cannot both be provided")
		}

		if userEmail == "" {
			userEmail = args[0]
		}

		if teamReference == "" {
			// Removing from organization
			if !force {
				confirmed, err := confirmRemoval(fmt.Sprintf("Are you sure you want to remove user '%s' from organization '%s'? This will also remove them from all teams in this organization. Type 'y' to confirm: ", userEmail, orgReference))
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintf(cmd.OutOrStdout(), "Operation cancelled\n")
					return nil
				}
			}

			err = removeOrgMember(cmd.Context(), apiClient, orgReference, userEmail)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "User '%s' removed from organization %s\n", userEmail, orgReference)
			return nil
		}

		// Removing from team only
		if !force {
			confirmed, err := confirmRemoval(fmt.Sprintf("Are you sure you want to remove user '%s' from team '%s'? Type 'y' to confirm: ", userEmail, teamReference))
			if err != nil {
				return err
			}
			if !confirmed {
				fmt.Fprintf(cmd.OutOrStdout(), "Operation cancelled\n")
				return nil
			}
		}

		err = removeTeamMember(cmd.Context(), apiClient, orgReference, teamReference, userEmail)
		if err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "User '%s' removed from team %s\n", userEmail, teamReference)
		return nil
	},
}

func confirmRemoval(prompt string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)
	fmt.Print(prompt)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

func removeOrgMember(ctx context.Context, apiClient *client.HelixClient, orgReference, userReference string) error {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return fmt.Errorf("failed to lookup organization: %w", err)
	}

	// First we need to get the user ID from the email/reference
	members, err := apiClient.ListOrganizationMembers(ctx, org.ID)
	if err != nil {
		return fmt.Errorf("failed to list organization members: %w", err)
	}

	var userID string
	for _, member := range members {
		if member.User.Email == userReference || member.User.ID == userReference || member.UserID == userReference {
			userID = member.UserID
			break
		}
	}

	if userID == "" {
		return fmt.Errorf("user not found in organization: %s", userReference)
	}

	err = apiClient.RemoveOrganizationMember(ctx, org.ID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove organization member: %w", err)
	}

	return nil
}

func removeTeamMember(ctx context.Context, apiClient *client.HelixClient, orgReference, teamReference, userReference string) error {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return fmt.Errorf("failed to lookup organization: %w", err)
	}

	team, err := lookupTeam(ctx, apiClient, org.ID, teamReference)
	if err != nil {
		return fmt.Errorf("failed to lookup team: %w", err)
	}

	// First we need to get the user ID from the email/reference
	members, err := apiClient.ListTeamMembers(ctx, org.ID, team.ID)
	if err != nil {
		return fmt.Errorf("failed to list team members: %w", err)
	}

	var userID string
	for _, member := range members {
		if member.User.Email == userReference || member.User.ID == userReference || member.UserID == userReference {
			userID = member.UserID
			break
		}
	}

	if userID == "" {
		return fmt.Errorf("user not found in team: %s", userReference)
	}

	err = apiClient.RemoveTeamMember(ctx, org.ID, team.ID, userID)
	if err != nil {
		return fmt.Errorf("failed to remove team member: %w", err)
	}

	return nil
}
