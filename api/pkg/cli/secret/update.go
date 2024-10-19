package secret

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().StringP("name", "n", "", "Name of the secret")
	updateCmd.Flags().StringP("value", "v", "", "New value of the secret")
	_ = updateCmd.MarkFlagRequired("name")
	_ = updateCmd.MarkFlagRequired("value")
}

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update a secret value",
	Long:  `Update an existing secret by providing its name and updating its value.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")

		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("secret name cannot be empty")
		}

		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("secret value cannot be empty")
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		var existingSecret types.Secret

		// Fetch the existing secret
		secrets, err := apiClient.ListSecrets()
		if err != nil {
			return fmt.Errorf("failed to fetch existing secret: %w", err)
		}

		for _, secret := range secrets {
			if secret.Name == name {
				existingSecret = *secret
				break
			}
		}

		if existingSecret.ID == "" {
			return fmt.Errorf("secret with name %s not found", name)
		}

		// Update the secret fields if provided
		existingSecret.Value = []byte(value)

		_, err = apiClient.UpdateSecret(existingSecret.ID, &existingSecret)
		if err != nil {
			return fmt.Errorf("failed to update secret: %w", err)
		}

		fmt.Printf("Secret updated successfully\n")
		return nil
	},
}
