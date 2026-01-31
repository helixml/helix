package user

import (
	"context"
	"fmt"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(resetPasswordCmd)
}

var resetPasswordCmd = &cobra.Command{
	Use:   "reset-password <email> <new-password>",
	Short: "Reset a user's password",
	Long: `Reset a user's password.

Admins can reset any user's password by specifying the user's email.
Non-admin users can only reset their own password (email must match their account).

Examples:
  # Admin resetting another user's password
  helix user reset-password user@example.com newpassword123

  # User resetting their own password
  helix user reset-password myemail@example.com mynewpassword`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		email := args[0]
		newPassword := args[1]

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		return resetPassword(cmd.Context(), apiClient, email, newPassword, cmd)
	},
}

func resetPassword(ctx context.Context, apiClient *client.HelixClient, email, newPassword string, cmd *cobra.Command) error {
	// Get the current user to check if they're an admin
	currentUser, err := apiClient.GetCurrentUser(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user: %w", err)
	}

	// Check if user is resetting their own password
	if currentUser.Email == email {
		// User is resetting their own password
		err = apiClient.UpdateOwnPassword(ctx, newPassword)
		if err != nil {
			return fmt.Errorf("failed to update password: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Password updated successfully for %s\n", email)
		return nil
	}

	// User is trying to reset someone else's password - must be admin
	if !currentUser.Admin {
		return fmt.Errorf("only admins can reset other users' passwords")
	}

	// Look up the target user by email
	users, err := apiClient.ListUsers(ctx, &client.UserFilter{
		Email:   email,
		PerPage: 1,
	})
	if err != nil {
		return fmt.Errorf("failed to look up user: %w", err)
	}

	if len(users.Users) == 0 {
		return fmt.Errorf("user not found: %s", email)
	}

	targetUser := users.Users[0]

	// Reset the password using admin endpoint
	_, err = apiClient.AdminResetPassword(ctx, targetUser.ID, newPassword)
	if err != nil {
		return fmt.Errorf("failed to reset password: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Password reset successfully for %s\n", email)
	return nil
}
