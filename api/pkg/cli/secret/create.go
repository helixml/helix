package secret

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringP("name", "n", "", "Name of the secret")
	createCmd.Flags().StringP("value", "v", "", "Value of the secret")
	createCmd.Flags().StringP("app-id", "a", "", "App ID to associate the secret with")
	_ = createCmd.MarkFlagRequired("name")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new secret",
	Long:  `Create a new secret with a name and value.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")
		appID, _ := cmd.Flags().GetString("app-id")

		if value == "" {
			fmt.Print("Enter secret value (input will be hidden): ")
			byteValue, err := term.ReadPassword(int(os.Stdin.Fd()))
			if err != nil {
				return fmt.Errorf("failed to read secret value: %w", err)
			}
			value = string(byteValue)
			fmt.Println() // Print a newline after the hidden input
		}

		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}

		secret := &types.Secret{
			Name:  name,
			Value: []byte(value),
			AppID: appID,
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
