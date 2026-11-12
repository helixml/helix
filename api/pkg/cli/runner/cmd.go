// Package runner holds the `helix runner` CLI commands.
package runner

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/cli"
	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

var rootCmd = &cobra.Command{
	Use:     "runner",
	Short:   "Manage runner (sandbox host) credentials",
	Aliases: []string{"runners"},
}

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage per-runner tokens",
	Long: `Per-runner tokens replace the shared RUNNER_TOKEN on sandbox hosts.

A token is bound to one runner id (the host's SANDBOX_INSTANCE_ID) and can
only register that host's RevDial tunnels and call its own sandbox and runner
routes. Set it as the host's RUNNER_TOKEN; nothing else on the host changes.
All commands require admin privileges.`,
	Aliases: []string{"tokens"},
}

var (
	createRunnerID string
	createName     string
	listRunnerID   string
)

func init() {
	createCmd.Flags().StringVar(&createRunnerID, "runner-id", "", "Runner id the token is bound to (the host's SANDBOX_INSTANCE_ID)")
	createCmd.Flags().StringVar(&createName, "name", "", "Optional label, e.g. the host name")
	_ = createCmd.MarkFlagRequired("runner-id")
	listCmd.Flags().StringVar(&listRunnerID, "runner-id", "", "Only list this runner's tokens")

	tokenCmd.AddCommand(createCmd, listCmd, revokeCmd)
	rootCmd.AddCommand(tokenCmd)
}

// New returns the `helix runner` command.
func New() *cobra.Command {
	return rootCmd
}

var createCmd = &cobra.Command{
	Use:   "create --runner-id <id>",
	Short: "Mint a token bound to one runner id",
	Long: `Mint a per-runner token. The token is printed once and cannot be shown
again: only its hash is stored.

Example:
  helix runner token create --runner-id gpu-host-1 --name "gpu-host-1 (rack 3)"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}
		created, err := apiClient.CreateRunnerToken(cmd.Context(), &types.CreateRunnerTokenRequest{
			RunnerID: createRunnerID,
			Name:     createName,
		})
		if err != nil {
			return fmt.Errorf("failed to create runner token: %w", err)
		}
		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Created runner token %s for runner %s.\n", created.ID, created.RunnerID)
		fmt.Fprintf(out, "Set this as the host's RUNNER_TOKEN. It will not be shown again:\n\n%s\n", created.Token)
		return nil
	},
}

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List per-runner tokens (never their secrets)",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}
		tokens, err := apiClient.ListRunnerTokens(cmd.Context(), listRunnerID)
		if err != nil {
			return fmt.Errorf("failed to list runner tokens: %w", err)
		}
		table := cli.NewSimpleTable(cmd.OutOrStdout(), []string{"ID", "Runner ID", "Name", "Prefix", "Created", "Revoked"})
		for _, t := range tokens {
			revoked := ""
			if t.RevokedAt != nil {
				revoked = t.RevokedAt.Format(time.RFC3339)
			}
			cli.AppendRow(table, []string{t.ID, t.RunnerID, t.Name, t.TokenPrefix, t.CreatedAt.Format(time.RFC3339), revoked})
		}
		cli.RenderTable(table)
		return nil
	},
}

var revokeCmd = &cobra.Command{
	Use:     "revoke <token-id>",
	Aliases: []string{"rm", "delete"},
	Short:   "Revoke a per-runner token",
	Long: `Revoke a per-runner token. New requests with it are refused at once; a
tunnel it already opened stays up until the host reconnects.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		apiClient, err := client.NewClientFromEnv()
		if err != nil {
			return err
		}
		revoked, err := apiClient.RevokeRunnerToken(cmd.Context(), args[0])
		if err != nil {
			return fmt.Errorf("failed to revoke runner token: %w", err)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Revoked runner token %s (runner %s).\n", revoked.ID, revoked.RunnerID)
		return nil
	},
}
