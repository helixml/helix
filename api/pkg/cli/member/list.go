package member

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	listCmd.Flags().StringP("organization", "o", "", "Organization ID or name")

	if err := listCmd.MarkFlagRequired("organization"); err != nil {
		return
	}

	listCmd.Flags().StringP("team", "t", "", "Team ID or name, supply to list members of a specific team")

	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List members",
	Long:    ``,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		orgReference, _ := cmd.Flags().GetString("organization")
		teamReference, _ := cmd.Flags().GetString("team")

		members, err := getMembers(cmd.Context(), apiClient, orgReference, teamReference)
		if err != nil {
			return fmt.Errorf("failed to get members: %w", err)
		}

		header := []string{"User ID", "Email", "Name"}

		if teamReference == "" {
			header = append(header, "Role", "Teams")
		}

		table := cli.NewSimpleTable(cmd.OutOrStdout(), header)

		for _, m := range members {
			row := []string{
				m.UserID,
				m.Email,
				m.Name,
			}

			if teamReference == "" {
				// If we are showing organization members, show the role and the teams they are a part of
				row = append(row, m.Role)
				row = append(row, strings.Join(m.Teams, ", "))
			}

			cli.AppendRow(table, row)
		}

		cli.RenderTable(table)

		return nil
	},
}

type member struct {
	UserID string
	Email  string
	Name   string
	Role   string
	Teams  []string
}

func getMembers(ctx context.Context, apiClient *client.HelixClient, orgReference, teamReference string) ([]*member, error) {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup organization: %w", err)
	}

	var members []*member

	// Showing organization members
	if teamReference == "" {
		// Just getting org members
		orgMembers, err := apiClient.ListOrganizationMembers(ctx, org.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list organization members: %w", err)
		}

		teams, err := apiClient.ListTeams(ctx, org.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list organization teams: %w", err)
		}

		for _, m := range orgMembers {
			members = append(members, &member{
				UserID: m.User.ID,
				Email:  m.User.Email,
				Name:   m.User.FullName,
				Role:   string(m.Role),
				Teams:  populateMemberTeams(m, teams),
			})
		}

		return members, nil
	}

	team, err := lookupTeam(ctx, apiClient, org.ID, teamReference)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup team: %w", err)
	}

	teamMembers, err := apiClient.ListTeamMembers(ctx, org.ID, team.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list team members: %w", err)
	}

	for _, m := range teamMembers {
		members = append(members, &member{
			UserID: m.User.ID,
			Email:  m.User.Email,
			Name:   m.User.FullName,
		})
	}

	return members, nil
}

func populateMemberTeams(orgMember *types.OrganizationMembership, teams []*types.Team) []string {
	// Team names that the org member is a part of
	memberTeams := []string{}

	for _, t := range teams {
		for _, mt := range t.Memberships {
			if mt.UserID == orgMember.UserID {
				memberTeams = append(memberTeams, t.Name)
			}
		}
	}

	return memberTeams
}
