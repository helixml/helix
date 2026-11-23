package secret

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

func init() {
	rootCmd.AddCommand(createCmd)
	createCmd.Flags().StringP("name", "n", "", "Name of the secret")
	createCmd.Flags().StringP("value", "v", "", "Value of the secret")
	createCmd.Flags().StringP("app-id", "a", "", "App ID to associate the secret with")
	createCmd.Flags().StringP("project", "p", "", "Project ID or name to associate the secret with (injected as env var in project sessions)")
	createCmd.Flags().String("scope", string(types.SecretScopeDev), "Project secret environment: dev (project sessions), prod (web services), or both")
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
		scopeValue, _ := cmd.Flags().GetString("scope")
		scope := types.SecretScope(scopeValue)
		if !scope.Valid() {
			return fmt.Errorf("invalid secret scope %q, must be one of: dev, prod, both", scopeValue)
		}
		if projectID == "" && cmd.Flags().Changed("scope") {
			return fmt.Errorf("--scope requires --project")
		}

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
			Name:  name,
			Value: value,
			AppID: appID,
			Scope: string(scope),
		}

		if projectID != "" {
			project, resolveErr := cli.LookupProject(cmd.Context(), apiClient, projectID, "")
			if resolveErr != nil {
				return resolveErr
			}
			projectID = project.ID
			_, err = apiClient.CreateProjectSecret(cmd.Context(), projectID, secret)
		} else {
			_, err = apiClient.CreateSecret(cmd.Context(), secret)
		}
		if err != nil {
			return fmt.Errorf("failed to create secret: %w", err)
		}

		if projectID != "" {
			target := "project sessions"
			switch scope {
			case types.SecretScopeProd:
				target = "web services"
			case types.SecretScopeBoth:
				target = "project sessions and web services"
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Secret '%s' created for project %s with %s scope (injected into %s)\n", name, projectID, scope, target)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "Secret created successfully")
		}

		return nil
	},
}
