package user

import (
	"context"
	"fmt"
	"text/tabwriter"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/spf13/cobra"
)

var (
	listEmail    string
	listUsername string
	listPage     int
	listPerPage  int
)

func init() {
	listCmd.Flags().StringVar(&listEmail, "email", "", "Filter by email address")
	listCmd.Flags().StringVar(&listUsername, "username", "", "Filter by username")
	listCmd.Flags().IntVar(&listPage, "page", 1, "Page number")
	listCmd.Flags().IntVar(&listPerPage, "per-page", 50, "Results per page (max 200)")
	rootCmd.AddCommand(listCmd)
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List users",
	Long: `List all users in Helix with their admin status.

This command requires admin privileges.

Examples:
  # List all users
  helix user list

  # Filter by email
  helix user list --email user@example.com

  # Filter by username
  helix user list --username john

  # Pagination
  helix user list --page 2 --per-page 20`,
	Aliases: []string{"ls"},
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		return listUsers(cmd.Context(), apiClient, cmd)
	},
}

func listUsers(ctx context.Context, apiClient *client.HelixClient, cmd *cobra.Command) error {
	// Get current user status (for admin check - uses server's config-based admin determination)
	status, err := apiClient.GetCurrentUserStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user status: %w", err)
	}

	// Print results in a table format
	w := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tEMAIL\tUSERNAME\tFULL NAME\tADMIN\tDEACTIVATED")
	fmt.Fprintln(w, "--\t-----\t--------\t---------\t-----\t-----------")

	if !status.Admin {
		// Non-admin users can only see their own account
		// Fetch full user details for display
		currentUser, err := apiClient.GetUser(ctx, status.User)
		if err != nil {
			return fmt.Errorf("failed to get user details: %w", err)
		}
		printUser(w, currentUser)
		w.Flush()
		return nil
	}

	// Admin users can see all users
	filter := &client.UserFilter{
		Email:    listEmail,
		Username: listUsername,
		Page:     listPage,
		PerPage:  listPerPage,
	}

	result, err := apiClient.ListUsers(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list users: %w", err)
	}

	if len(result.Users) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No users found")
		return nil
	}

	for _, user := range result.Users {
		printUser(w, user)
	}
	w.Flush()

	// Print pagination info
	fmt.Fprintf(cmd.OutOrStdout(), "\nPage %d of %d (total: %d users)\n",
		result.Page, result.TotalPages, result.TotalCount)

	return nil
}

func printUser(w *tabwriter.Writer, user *types.User) {
	adminStatus := "no"
	if user.Admin {
		adminStatus = "yes"
	}
	deactivatedStatus := "no"
	if user.Deactivated {
		deactivatedStatus = "yes"
	}
	fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
		user.ID,
		user.Email,
		user.Username,
		user.FullName,
		adminStatus,
		deactivatedStatus,
	)
}
