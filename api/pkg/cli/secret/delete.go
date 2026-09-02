package secret

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().StringP("name", "n", "", "Name of the secret to delete")
	deleteCmd.Flags().StringP("project", "p", "", "Project ID or name containing the secret")
}

var deleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"rm"},
	Short:   "Delete a secret by name",
	Long:    `Delete an existing secret by providing its name.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Look for either name or first arg
		name, _ := cmd.Flags().GetString("name")
		projectRef, _ := cmd.Flags().GetString("project")
		if name == "" {
			if len(args) > 0 {
				name = args[0]
			}
		}

		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("secret name cannot be empty")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var secrets []*types.Secret
		if projectRef != "" {
			project, resolveErr := cli.LookupProject(cmd.Context(), apiClient, projectRef, "")
			if resolveErr != nil {
				return resolveErr
			}
			secrets, err = apiClient.ListProjectSecrets(cmd.Context(), project.ID)
		} else {
			secrets, err = apiClient.ListSecrets(cmd.Context(), nil)
		}
		if err != nil {
			return fmt.Errorf("failed to fetch secrets: %w", err)
		}

		// Find the secret with the given name
		var secretID string
		for _, secret := range secrets {
			if secret.Name == name {
				secretID = secret.ID
				break
			}
		}

		if secretID == "" {
			return fmt.Errorf("secret with name %s not found", name)
		}

		// Delete the secret
		err = apiClient.DeleteSecret(cmd.Context(), secretID)
		if err != nil {
			return fmt.Errorf("failed to delete secret: %w", err)
		}

		fmt.Printf("Secret '%s' deleted successfully\n", name)
		return nil
	},
}
