package user

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/spf13/cobra"
)

var (
	deleteForce bool
)

func init() {
	deleteCmd.Flags().BoolVarP(&deleteForce, "force", "f", false, "Skip confirmation prompt")
	rootCmd.AddCommand(deleteCmd)
}

var deleteCmd = &cobra.Command{
	Use:   "delete <user-id>",
	Short: "Delete a user",
	Long: `Permanently delete a user and all associated data.

This command requires admin privileges.

WARNING: This action is irreversible. The user and all their data
(API keys, user metadata, etc.) will be permanently deleted.

Examples:
  # Delete a user (with confirmation prompt)
  helix user delete abc123

  # Delete a user without confirmation
  helix user delete abc123 --force`,
	Aliases: []string{"rm"},
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return fmt.Errorf("failed to create client: %w", err)
		}

		return deleteUser(cmd.Context(), apiClient, args[0], deleteForce)
	},
}

func deleteUser(ctx context.Context, apiClient *client.HelixClient, userID string, force bool) error {
	// Get current user status to verify admin permissions
	status, err := apiClient.GetCurrentUserStatus(ctx)
	if err != nil {
		return fmt.Errorf("failed to get current user status: %w", err)
	}

	if !status.Admin {
		return fmt.Errorf("only admins can delete users")
	}

	// Prevent deleting yourself
	if status.User == userID {
		return fmt.Errorf("cannot delete your own account")
	}

	// Get target user details for confirmation
	targetUser, err := apiClient.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user details: %w", err)
	}

	// Confirmation prompt (unless --force is used)
	if !force {
		fmt.Printf("You are about to permanently delete the following user:\n")
		fmt.Printf("  ID:       %s\n", targetUser.ID)
		fmt.Printf("  Email:    %s\n", targetUser.Email)
		fmt.Printf("  Username: %s\n", targetUser.Username)
		fmt.Printf("  Name:     %s\n", targetUser.FullName)
		fmt.Printf("\nThis action is IRREVERSIBLE. All user data will be permanently deleted.\n")
		fmt.Printf("\nType 'yes' to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read confirmation: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "yes" {
			fmt.Println("Deletion cancelled.")
			return nil
		}
	}

	// Perform deletion
	err = apiClient.AdminDeleteUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}

	fmt.Printf("User %s (%s) has been permanently deleted.\n", targetUser.Email, userID)
	return nil
}
