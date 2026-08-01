package org

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/spf13/cobra"
)

func newAssetsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "assets",
		Aliases: []string{"asset"},
		Short:   "Manage helix-org assets and agent access",
	}
	cmd.AddCommand(newAssetsListCmd())
	cmd.AddCommand(newAssetsGetCmd())
	cmd.AddCommand(newAssetsCreateCmd())
	cmd.AddCommand(newAssetsUpdateCmd())
	cmd.AddCommand(newAssetsHealthCmd())
	cmd.AddCommand(newAssetsLinkCmd(false))
	cmd.AddCommand(newAssetsLinkCmd(true))
	cmd.AddCommand(newAssetsDeleteCmd())
	return cmd
}

func addOrgFlag(cmd *cobra.Command, value *string) {
	cmd.Flags().StringVar(value, "org", "", "Organization id or name (or $HELIX_ORG)")
}

func resolveAssetOrg(cmd *cobra.Command, orgFlag string) (*httpClient, string, error) {
	c, err := newHTTPClient()
	if err != nil {
		return nil, "", err
	}
	orgID, err := c.resolveOrg(cmd.Context(), orgFlag)
	if err != nil {
		return nil, "", err
	}
	if orgID == "" {
		return nil, "", fmt.Errorf("organization is required")
	}
	return c, orgID, nil
}

func newAssetsListCmd() *cobra.Command {
	var orgFlag string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List assets in an organization",
		RunE: func(cmd *cobra.Command, _ []string) error {
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			var response orgapi.AssetsResponse
			if err := c.doJSON(cmd.Context(), http.MethodGet, "/orgs/"+orgID+"/assets", nil, &response, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(response)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%-38s %-20s %-10s %-28s %s\n", "ID", "NAME", "KIND", "ENDPOINT", "AGENTS")
			for _, value := range response.Assets {
				endpoint := "-"
				if value.Server != nil {
					endpoint = fmt.Sprintf("%s@%s:%d", value.Server.User, value.Server.Address, value.Server.Port)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-38s %-20s %-10s %-28s %d\n", value.ID, truncate(value.Name, 20), value.Kind, truncate(endpoint, 28), len(value.AgentIDs))
			}
			if len(response.Assets) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "(none)")
			}
			return nil
		},
	}
	addOrgFlag(cmd, &orgFlag)
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newAssetsGetCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "get <asset-id>",
		Short: "Get one asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			var value orgapi.AssetDTO
			if err := c.doJSON(cmd.Context(), http.MethodGet, fmt.Sprintf("/orgs/%s/assets/%s", orgID, args[0]), nil, &value, 30*time.Second); err != nil {
				return err
			}
			return printJSON(value)
		},
	}
	addOrgFlag(cmd, &orgFlag)
	return cmd
}

func newAssetsCreateCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "create", Short: "Create an asset"}
	cmd.AddCommand(newAssetsCreateServerCmd())
	return cmd
}

func newAssetsCreateServerCmd() *cobra.Command {
	var orgFlag, address, user, auth, description, notes, hostKey string
	var port uint16
	var passwordStdin, jsonOut bool
	cmd := &cobra.Command{
		Use:   "server <name>",
		Short: "Create a server asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			authType, err := parseAssetAuthType(auth)
			if err != nil {
				return err
			}
			password, err := readAssetPassword(cmd.InOrStdin(), authType, passwordStdin)
			if err != nil {
				return err
			}
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			request := orgapi.CreateAssetRequest{
				Name: args[0], Description: description, NotesForAgents: notes, Kind: asset.KindServer,
				Server: &orgapi.ServerAssetWriteRequest{
					Address: address, Port: port, User: user, AuthType: authType,
					Password: password, HostKey: hostKey,
				},
			}
			var value orgapi.AssetDTO
			if err := c.doJSON(cmd.Context(), http.MethodPost, "/orgs/"+orgID+"/assets", request, &value, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(value)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created asset %s (%s)\n", value.Name, value.ID)
			if value.Server != nil && value.Server.PublicKey != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "Helix public key:")
				fmt.Fprintln(cmd.OutOrStdout(), value.Server.PublicKey)
			}
			return nil
		},
	}
	addOrgFlag(cmd, &orgFlag)
	cmd.Flags().StringVar(&address, "address", "", "Server IP address or hostname")
	cmd.Flags().Uint16Var(&port, "port", 22, "SSH port")
	cmd.Flags().StringVar(&user, "user", "", "SSH username")
	cmd.Flags().StringVar(&auth, "auth", "ssh-key", "Authentication: ssh-key or password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read the server password from stdin")
	cmd.Flags().StringVar(&description, "description", "", "Asset description")
	cmd.Flags().StringVar(&notes, "notes-for-agents", "", "Operational notes visible to linked agents")
	cmd.Flags().StringVar(&hostKey, "host-key", "", "Expected OpenSSH host public key")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	_ = cmd.MarkFlagRequired("address")
	_ = cmd.MarkFlagRequired("user")
	return cmd
}

func newAssetsUpdateCmd() *cobra.Command {
	var orgFlag, name, address, user, auth, description, notes, hostKey string
	var port uint16
	var passwordStdin, jsonOut bool
	cmd := &cobra.Command{
		Use:   "update <asset-id>",
		Short: "Update a server asset",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := orgapi.UpdateAssetRequest{}
			server := orgapi.UpdateServerAssetRequest{}
			serverChanged := false
			if cmd.Flags().Changed("name") {
				request.Name = &name
			}
			if cmd.Flags().Changed("description") {
				request.Description = &description
			}
			if cmd.Flags().Changed("notes-for-agents") {
				request.NotesForAgents = &notes
			}
			if cmd.Flags().Changed("address") {
				server.Address, serverChanged = &address, true
			}
			if cmd.Flags().Changed("port") {
				server.Port, serverChanged = &port, true
			}
			if cmd.Flags().Changed("user") {
				server.User, serverChanged = &user, true
			}
			if cmd.Flags().Changed("host-key") {
				server.HostKey, serverChanged = &hostKey, true
			}
			var authType asset.AuthType
			if cmd.Flags().Changed("auth") {
				var err error
				authType, err = parseAssetAuthType(auth)
				if err != nil {
					return err
				}
				server.AuthType, serverChanged = &authType, true
			}
			if passwordStdin {
				password, err := readAssetPassword(cmd.InOrStdin(), asset.AuthPassword, true)
				if err != nil {
					return err
				}
				server.Password, serverChanged = &password, true
			}
			if serverChanged {
				request.Server = &server
			}
			if request.Name == nil && request.Description == nil && request.NotesForAgents == nil && request.Server == nil {
				return fmt.Errorf("at least one update flag is required")
			}
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			var value orgapi.AssetDTO
			if err := c.doJSON(cmd.Context(), http.MethodPatch, fmt.Sprintf("/orgs/%s/assets/%s", orgID, args[0]), request, &value, 30*time.Second); err != nil {
				return err
			}
			if jsonOut {
				return printJSON(value)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Updated asset %s (%s)\n", value.Name, value.ID)
			if value.Server != nil && value.Server.PublicKey != "" && authType == asset.AuthSSHKey {
				fmt.Fprintln(cmd.OutOrStdout(), "New Helix public key:")
				fmt.Fprintln(cmd.OutOrStdout(), value.Server.PublicKey)
			}
			return nil
		},
	}
	addOrgFlag(cmd, &orgFlag)
	cmd.Flags().StringVar(&name, "name", "", "Asset name")
	cmd.Flags().StringVar(&description, "description", "", "Asset description")
	cmd.Flags().StringVar(&notes, "notes-for-agents", "", "Operational notes visible to linked agents")
	cmd.Flags().StringVar(&address, "address", "", "Server IP address or hostname")
	cmd.Flags().Uint16Var(&port, "port", 0, "SSH port")
	cmd.Flags().StringVar(&user, "user", "", "SSH username")
	cmd.Flags().StringVar(&auth, "auth", "", "Authentication: ssh-key or password")
	cmd.Flags().BoolVar(&passwordStdin, "password-stdin", false, "Read a replacement server password from stdin")
	cmd.Flags().StringVar(&hostKey, "host-key", "", "Expected OpenSSH host public key; empty clears it")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newAssetsHealthCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:   "health <asset-id>",
		Short: "Check network and Helix SSH connectivity",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			var health orgapi.AssetHealthDTO
			if err := c.doJSON(cmd.Context(), http.MethodGet, fmt.Sprintf("/orgs/%s/assets/%s/health", orgID, args[0]), nil, &health, 30*time.Second); err != nil {
				return err
			}
			return printJSON(health)
		},
	}
	addOrgFlag(cmd, &orgFlag)
	return cmd
}

func newAssetsLinkCmd(unlink bool) *cobra.Command {
	var orgFlag string
	verb := "link"
	short := "Allow an agent to use an asset"
	method := http.MethodPost
	if unlink {
		verb, short, method = "unlink", "Revoke an agent's access to an asset", http.MethodDelete
	}
	cmd := &cobra.Command{
		Use:   verb + " <asset-id> <agent-id>",
		Short: short,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			path := fmt.Sprintf("/orgs/%s/assets/%s/links", orgID, args[0])
			var body any = orgapi.AssetLinkRequest{AgentID: args[1]}
			if unlink {
				path += "/" + args[1]
				body = nil
			}
			if err := c.doJSON(cmd.Context(), method, path, body, nil, 30*time.Second); err != nil {
				return err
			}
			if unlink {
				fmt.Fprintf(cmd.OutOrStdout(), "Unlinked asset %s from agent %s\n", args[0], args[1])
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Linked asset %s to agent %s\n", args[0], args[1])
			}
			return nil
		},
	}
	addOrgFlag(cmd, &orgFlag)
	return cmd
}

func newAssetsDeleteCmd() *cobra.Command {
	var orgFlag string
	cmd := &cobra.Command{
		Use:     "delete <asset-id>",
		Aliases: []string{"rm"},
		Short:   "Delete an asset and revoke all agent access",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, orgID, err := resolveAssetOrg(cmd, orgFlag)
			if err != nil {
				return err
			}
			if err := c.doJSON(cmd.Context(), http.MethodDelete, fmt.Sprintf("/orgs/%s/assets/%s", orgID, args[0]), nil, nil, 30*time.Second); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Deleted asset %s\n", args[0])
			return nil
		},
	}
	addOrgFlag(cmd, &orgFlag)
	return cmd
}

func parseAssetAuthType(value string) (asset.AuthType, error) {
	switch strings.TrimSpace(value) {
	case "ssh-key", asset.AuthSSHKey:
		return asset.AuthSSHKey, nil
	case asset.AuthPassword:
		return asset.AuthPassword, nil
	default:
		return "", fmt.Errorf("unsupported authentication %q: use ssh-key or password", value)
	}
}

func readAssetPassword(input io.Reader, authType asset.AuthType, passwordStdin bool) (string, error) {
	if authType != asset.AuthPassword {
		if passwordStdin {
			return "", fmt.Errorf("--password-stdin requires --auth password")
		}
		return "", nil
	}
	if !passwordStdin {
		return "", fmt.Errorf("password authentication requires --password-stdin")
	}
	password, err := bufio.NewReader(input).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read password from stdin: %w", err)
	}
	password = strings.TrimSpace(password)
	if password == "" {
		return "", fmt.Errorf("password from stdin is empty")
	}
	return password, nil
}
