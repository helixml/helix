// Package identity holds `helix identity`: identity attestations that prove to
// an external verifier which spec task, session or sandbox the caller is.
package identity

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/helixml/helix/api/pkg/client"
	"github.com/helixml/helix/api/pkg/types"
)

// New returns the `helix identity` command.
func New() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "identity",
		Short: "Prove which spec task, session or sandbox you are to an external verifier",
	}
	cmd.AddCommand(newAttestCommand(), newVerifyCommand())
	return cmd
}

func newAttestCommand() *cobra.Command {
	var audience, nonce string
	cmd := &cobra.Command{
		Use:   "attest",
		Short: "Mint a single-use identity attestation and print the token",
		Long: `Mints a token, valid for 5 minutes, stating this desktop's spec task, session
or sandbox, project, organization and owner. Run it inside a Helix desktop or
sandbox (it authenticates with USER_API_TOKEN / HELIX_API_URL there). Hand the
token to the verifier that issued the nonce; never hand over the API key.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			resp, err := apiClient.CreateIdentityAttestation(cmd.Context(), &types.IdentityAttestationRequest{
				Audience: audience,
				Nonce:    nonce,
			})
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), resp.Token)
			return err
		},
	}
	cmd.Flags().StringVar(&audience, "audience", "", "Verifier the token is for, e.g. ptaas.example.com")
	cmd.Flags().StringVar(&nonce, "nonce", "", "Challenge issued by the verifier (16-128 characters)")
	_ = cmd.MarkFlagRequired("audience")
	_ = cmd.MarkFlagRequired("nonce")
	return cmd
}

func newVerifyCommand() *cobra.Command {
	var audience, nonce, tokenFile string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Redeem an identity attestation once and print its claims",
		Long: `Verifies a token minted by 'helix identity attest'. Authenticate with an
org-scoped API key or as a user who may read the attested project. The token
is read from --token-file (or stdin with "-") so it stays out of argv.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			token, err := readToken(cmd, tokenFile)
			if err != nil {
				return err
			}
			apiClient, err := client.NewClientFromEnv()
			if err != nil {
				return err
			}
			claims, err := apiClient.VerifyIdentityAttestation(cmd.Context(), &types.IdentityAttestationVerifyRequest{
				Token:    token,
				Audience: audience,
				Nonce:    nonce,
			})
			if err != nil {
				return err
			}
			enc := json.NewEncoder(cmd.OutOrStdout())
			enc.SetIndent("", "  ")
			return enc.Encode(claims)
		},
	}
	cmd.Flags().StringVar(&audience, "audience", "", "Audience the token was minted for")
	cmd.Flags().StringVar(&nonce, "nonce", "", "Nonce the token was minted for")
	cmd.Flags().StringVar(&tokenFile, "token-file", "-", `File holding the token, or "-" for stdin`)
	_ = cmd.MarkFlagRequired("audience")
	_ = cmd.MarkFlagRequired("nonce")
	return cmd
}

func readToken(cmd *cobra.Command, path string) (string, error) {
	var raw []byte
	var err error
	if path == "-" {
		raw, err = io.ReadAll(cmd.InOrStdin())
	} else {
		raw, err = os.ReadFile(path)
	}
	if err != nil {
		return "", fmt.Errorf("read token: %w", err)
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errors.New("no token given")
	}
	return token, nil
}
