package member

import (
	"context"
	"fmt"

	"github.com/olekukonko/tablewriter"
	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
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

		table := tablewriter.NewWriter(cmd.OutOrStdout())

		header := []string{"User ID", "Email", "Name"}

		table.SetHeader(header)

		table.SetAutoWrapText(false)
		table.SetAutoFormatHeaders(true)
		table.SetHeaderAlignment(tablewriter.ALIGN_LEFT)
		table.SetAlignment(tablewriter.ALIGN_LEFT)
		table.SetCenterSeparator("")
		table.SetColumnSeparator("")
		// table.SetRowSeparator("")
		table.SetHeaderLine(false)
		table.SetBorder(false)
		table.SetTablePadding(" ")
		table.SetNoWhiteSpace(false)

		for _, m := range members {
			row := []string{
				m.UserID,
				m.Email,
				m.Name,
			}

			table.Append(row)
		}

		table.Render()

		return nil
	},
}

type member struct {
	UserID string
	Email  string
	Name   string
}

func getMembers(ctx context.Context, apiClient *client.HelixClient, orgReference, teamReference string) ([]*member, error) {
	org, err := lookupOrganization(ctx, apiClient, orgReference)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup organization: %w", err)
	}

	var members []*member

	if teamReference == "" {
		// Just getting org members
		orgMembers, err := apiClient.ListOrganizationMembers(ctx, org.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list organization members: %w", err)
		}

		for _, m := range orgMembers {
			members = append(members, &member{
				UserID: m.User.ID,
				Email:  m.User.Email,
				Name:   m.User.FullName,
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
