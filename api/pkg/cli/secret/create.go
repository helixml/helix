package secret

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringP("name", "n", "", "Name of the secret")
	createCmd.Flags().StringP("value", "v", "", "Value of the secret")
	createCmd.MarkFlagRequired("name")
	createCmd.MarkFlagRequired("value")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new secret",
	Long:  `Create a new secret with a name and value.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		secret := &types.Secret{
			Name:  name,
			Value: []byte(value),
		}

		createdSecret, err := apiClient.CreateSecret(secret)
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}

		fmt.Printf("Secret created successfully:\n")
		fmt.Printf("ID: %s\n", createdSecret.ID)
		fmt.Printf("Name: %s\n", createdSecret.Name)

		return nil
	},
}
