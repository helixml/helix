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
	createCmd.Flags().StringP("project", "p", "", "Project ID to associate the secret with (injected as env var in project sessions)")
	_ = createCmd.MarkFlagRequired("name")
}

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new secret",
	Long:  `Create a new secret with a name and value.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		name, _ := cmd.Flags().GetString("name")
		value, _ := cmd.Flags().GetString("value")
		appID, _ := cmd.Flags().GetString("app-id")
		projectID, _ := cmd.Flags().GetString("project")

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

		secret := &types.CreateSecretRequest{
			Name:      name,
			Value:     value,
			AppID:     appID,
			ProjectID: projectID,
		}

		_, err = apiClient.CreateSecret(cmd.Context(), secret)
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}

		if projectID != "" {
			fmt.Printf("Secret '%s' created for project %s (will be injected as env var)\n", name, projectID)
		} else {
			fmt.Printf("Secret created successfully\n")
		}

		return nil
	},
}
