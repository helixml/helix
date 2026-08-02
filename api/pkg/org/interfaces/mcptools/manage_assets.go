package mcptools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"

	assetapp "github.com/helixml/helix/api/pkg/org/application/assets"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

// Asset management tools expose the org-wide inventory to manager Bots. The
// existing assets.go tools deliberately remain link-scoped operational tools.

type managedServerView struct {
	Address            string         `json:"address"`
	Port               uint16         `json:"port"`
	User               string         `json:"user"`
	AuthType           asset.AuthType `json:"auth_type"`
	PublicKey          string         `json:"public_key,omitempty"`
	PasswordConfigured bool           `json:"password_configured"`
	HostKeyConfigured  bool           `json:"host_key_configured"`
}

type managedAssetView struct {
	ID             string             `json:"id"`
	OrganizationID string             `json:"organization_id"`
	Name           string             `json:"name"`
	Description    string             `json:"description,omitempty"`
	NotesForAgents string             `json:"notes_for_agents,omitempty"`
	Enabled        bool               `json:"enabled"`
	Kind           asset.Kind         `json:"kind"`
	Server         *managedServerView `json:"server,omitempty"`
	AgentIDs       []string           `json:"agent_ids"`
	CreatedAt      time.Time          `json:"created_at"`
	UpdatedAt      time.Time          `json:"updated_at"`
}

type serverAssetSetup struct {
	PublicKey            string `json:"public_key"`
	AuthorizedKeysPath   string `json:"authorized_keys_path"`
	InstallCommand       string `json:"install_command"`
	VerificationToolName string `json:"verification_tool"`
}

type managedAssetResult struct {
	Asset managedAssetView  `json:"asset"`
	Setup *serverAssetSetup `json:"setup,omitempty"`
}

type orgAssetsResult struct {
	Assets []managedAssetView `json:"assets"`
}

type assetLinksResult struct {
	AssetID  string   `json:"asset_id"`
	AgentIDs []string `json:"agent_ids"`
}

type assetMutationResult struct {
	AssetID string `json:"asset_id"`
	Status  string `json:"status"`
}

func listAssetLinksResult(ctx context.Context, deps Deps, orgID string, value asset.Asset) (assetLinksResult, error) {
	links, err := deps.Assets.ListLinks(ctx, orgID, value.ID)
	if err != nil {
		return assetLinksResult{}, err
	}
	ids := make([]string, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.AgentID)
	}
	return assetLinksResult{AssetID: value.ID, AgentIDs: ids}, nil
}

func managedAssetsOrgID(inv tool.Invocation, operation string) (string, error) {
	if inv.Caller == nil {
		return "", fmt.Errorf("%s: caller is missing", operation)
	}
	orgID := inv.Caller.OrganizationID()
	if orgID == "" {
		return "", fmt.Errorf("%s: caller has no organization ID", operation)
	}
	return orgID, nil
}

func managedAsset(t Deps, ctx context.Context, orgID string, value asset.Asset) (managedAssetView, error) {
	links, err := t.Assets.ListLinks(ctx, orgID, value.ID)
	if err != nil {
		return managedAssetView{}, err
	}
	agentIDs := make([]string, 0, len(links))
	for _, link := range links {
		agentIDs = append(agentIDs, link.AgentID)
	}
	view := managedAssetView{
		ID: value.ID, OrganizationID: orgID, Name: value.Name,
		Description: value.Description, NotesForAgents: value.NotesForAgents,
		Enabled: !value.Disabled, Kind: value.Kind, AgentIDs: agentIDs,
		CreatedAt: value.CreatedAt, UpdatedAt: value.UpdatedAt,
	}
	if value.Config.Server != nil {
		server := value.Config.Server
		view.Server = &managedServerView{
			Address: server.Address, Port: server.Port, User: server.User,
			AuthType: server.AuthType, PublicKey: server.PublicKey,
			PasswordConfigured: server.EncryptedPassword != "",
			HostKeyConfigured:  server.HostKey != "",
		}
	}
	return view, nil
}

func resolveManagedAsset(t Deps, ctx context.Context, orgID, ref string) (asset.Asset, error) {
	if t.Assets == nil {
		return asset.Asset{}, errors.New("assets service is not wired")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return asset.Asset{}, errors.New("asset is required")
	}
	value, err := t.Assets.Resolve(ctx, orgID, ref)
	if err != nil {
		return asset.Asset{}, err
	}
	return value, nil
}

func setupForServer(value asset.Asset) *serverAssetSetup {
	if value.Config.Server == nil || value.Config.Server.AuthType != asset.AuthSSHKey || value.Config.Server.PublicKey == "" {
		return nil
	}
	publicKey := strings.TrimSpace(value.Config.Server.PublicKey)
	encoded := base64.StdEncoding.EncodeToString([]byte(publicKey + "\n"))
	command := "install -d -m 700 \"$HOME/.ssh\" && touch \"$HOME/.ssh/authorized_keys\" && " +
		"chmod 600 \"$HOME/.ssh/authorized_keys\" && " +
		"(printf '%s' '" + encoded + "' | base64 -d | grep -qxFf - \"$HOME/.ssh/authorized_keys\" || " +
		"printf '%s' '" + encoded + "' | base64 -d >> \"$HOME/.ssh/authorized_keys\")"
	return &serverAssetSetup{
		PublicKey: publicKey, AuthorizedKeysPath: "$HOME/.ssh/authorized_keys",
		InstallCommand: command, VerificationToolName: string(GetAssetHealthName),
	}
}

// --- list_org_assets ------------------------------------------------------

const ListOrgAssetsName tool.Name = "list_org_assets"

type ListOrgAssets struct{ deps Deps }

func (t *ListOrgAssets) Name() tool.Name                 { return ListOrgAssetsName }
func (t *ListOrgAssets) InputSchema() *jsonschema.Schema { return mustSchema[struct{}]() }
func (t *ListOrgAssets) Description() string {
	return "List every asset in the organization, including linked agent IDs and public connection configuration. Unlike list_assets, this owner-management view is not limited to assets linked to the caller."
}
func (t *ListOrgAssets) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID, err := managedAssetsOrgID(inv, string(ListOrgAssetsName))
	if err != nil {
		return nil, err
	}
	if t.deps.Assets == nil {
		return nil, errors.New("assets service is not wired")
	}
	values, err := t.deps.Assets.List(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("list org assets: %w", err)
	}
	views := make([]managedAssetView, 0, len(values))
	for _, value := range values {
		view, err := managedAsset(t.deps, ctx, orgID, value)
		if err != nil {
			return nil, fmt.Errorf("list links for asset %q: %w", value.ID, err)
		}
		views = append(views, view)
	}
	return json.Marshal(orgAssetsResult{Assets: views})
}

// --- get_org_asset --------------------------------------------------------

const GetOrgAssetName tool.Name = "get_org_asset"

type GetOrgAsset struct{ deps Deps }

func (t *GetOrgAsset) Name() tool.Name                 { return GetOrgAssetName }
func (t *GetOrgAsset) InputSchema() *jsonschema.Schema { return mustSchema[assetRefArgs]() }
func (t *GetOrgAsset) Description() string {
	return "Get any org asset by ID or name, including linked agent IDs and public connection configuration."
}
func (t *GetOrgAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(GetOrgAssetName))
	if err != nil {
		return nil, err
	}
	value, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("get org asset: %w", err)
	}
	view, err := managedAsset(t.deps, ctx, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("get asset links: %w", err)
	}
	return json.Marshal(view)
}

// --- create_server_asset --------------------------------------------------

const CreateServerAssetName tool.Name = "create_server_asset"

type CreateServerAsset struct{ deps Deps }

type createServerAssetArgs struct {
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	NotesForAgents string         `json:"notes_for_agents,omitempty"`
	Address        string         `json:"address"`
	Port           uint16         `json:"port,omitempty"`
	User           string         `json:"user"`
	AuthType       asset.AuthType `json:"auth_type,omitempty"`
	Password       string         `json:"password,omitempty"`
	HostKey        string         `json:"host_key,omitempty"`
	AgentIDs       []string       `json:"agent_ids,omitempty"`
}

func (t *CreateServerAsset) Name() tool.Name { return CreateServerAssetName }
func (t *CreateServerAsset) InputSchema() *jsonschema.Schema {
	return mustSchema[createServerAssetArgs]()
}
func (t *CreateServerAsset) Description() string {
	return "Create a server asset and link it to the calling Bot in one action; agent_ids adds more links. auth_type defaults to ssh_key. SSH-key creation returns the Helix public key and an idempotent install_command. If you already have independent SSH access, run that command on the server; otherwise send it to the owner with ask_human. Then call get_asset_health and a server operation before claiming readiness."
}
func (t *CreateServerAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args createServerAssetArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(CreateServerAssetName))
	if err != nil {
		return nil, err
	}
	if t.deps.Assets == nil {
		return nil, errors.New("assets service is not wired")
	}
	value, err := t.deps.Assets.CreateServer(ctx, orgID, assetapp.CreateServerParams{
		Name: args.Name, Description: args.Description, NotesForAgents: args.NotesForAgents,
		Address: args.Address, Port: args.Port, User: args.User, AuthType: args.AuthType,
		Password: args.Password, HostKey: args.HostKey,
	})
	if err != nil {
		return nil, fmt.Errorf("create server asset: %w", err)
	}
	ids := append([]string{inv.Caller.ID()}, args.AgentIDs...)
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if _, err := t.deps.Assets.Link(ctx, orgID, value.ID, id); err != nil {
			if cleanupErr := t.deps.Assets.Delete(ctx, orgID, value.ID); cleanupErr != nil {
				return nil, fmt.Errorf("link new asset to agent %q: %w; rollback asset: %v", id, err, cleanupErr)
			}
			return nil, fmt.Errorf("link new asset to agent %q: %w", id, err)
		}
	}
	view, err := managedAsset(t.deps, ctx, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("read created asset: %w", err)
	}
	return json.Marshal(managedAssetResult{Asset: view, Setup: setupForServer(value)})
}

// --- update_server_asset --------------------------------------------------

const UpdateServerAssetName tool.Name = "update_server_asset"

type UpdateServerAsset struct{ deps Deps }

type updateServerAssetArgs struct {
	Asset          string          `json:"asset"`
	Name           *string         `json:"name,omitempty"`
	Description    *string         `json:"description,omitempty"`
	NotesForAgents *string         `json:"notes_for_agents,omitempty"`
	Enabled        *bool           `json:"enabled,omitempty"`
	Address        *string         `json:"address,omitempty"`
	Port           *uint16         `json:"port,omitempty"`
	User           *string         `json:"user,omitempty"`
	AuthType       *asset.AuthType `json:"auth_type,omitempty"`
	Password       *string         `json:"password,omitempty"`
	HostKey        *string         `json:"host_key,omitempty"`
}

func (t *UpdateServerAsset) Name() tool.Name { return UpdateServerAssetName }
func (t *UpdateServerAsset) InputSchema() *jsonschema.Schema {
	return mustSchema[updateServerAssetArgs]()
}
func (t *UpdateServerAsset) Description() string {
	return "Patch a server asset by ID or name. Only supplied fields change. Set enabled=false to block agent MCP and proxy SSH access without removing agent links. Switching to ssh_key generates a new key and returns its install_command; switching to password requires password."
}
func (t *UpdateServerAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args updateServerAssetArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(UpdateServerAssetName))
	if err != nil {
		return nil, err
	}
	current, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("resolve server asset: %w", err)
	}
	value, err := t.deps.Assets.UpdateServer(ctx, orgID, current.ID, assetapp.UpdateServerParams{
		Name: args.Name, Description: args.Description, NotesForAgents: args.NotesForAgents,
		Enabled: args.Enabled,
		Address: args.Address, Port: args.Port, User: args.User, AuthType: args.AuthType,
		Password: args.Password, HostKey: args.HostKey,
	})
	if err != nil {
		return nil, fmt.Errorf("update server asset: %w", err)
	}
	view, err := managedAsset(t.deps, ctx, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("read updated asset: %w", err)
	}
	return json.Marshal(managedAssetResult{Asset: view, Setup: setupForServer(value)})
}

// --- delete_asset ---------------------------------------------------------

const DeleteAssetName tool.Name = "delete_asset"

type DeleteAsset struct{ deps Deps }

func (t *DeleteAsset) Name() tool.Name                 { return DeleteAssetName }
func (t *DeleteAsset) InputSchema() *jsonschema.Schema { return mustSchema[assetRefArgs]() }
func (t *DeleteAsset) Description() string {
	return "Delete an org asset by ID or name. This also removes every agent link and its derived operational tools."
}
func (t *DeleteAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(DeleteAssetName))
	if err != nil {
		return nil, err
	}
	value, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("resolve asset: %w", err)
	}
	if err := t.deps.Assets.Delete(ctx, orgID, value.ID); err != nil {
		return nil, fmt.Errorf("delete asset: %w", err)
	}
	return json.Marshal(assetMutationResult{AssetID: value.ID, Status: "deleted"})
}

// --- asset links ----------------------------------------------------------

const ListAssetLinksName tool.Name = "list_asset_links"
const LinkAssetName tool.Name = "link_asset"
const UnlinkAssetName tool.Name = "unlink_asset"

type ListAssetLinks struct{ deps Deps }
type LinkAsset struct{ deps Deps }
type UnlinkAsset struct{ deps Deps }

type assetLinkArgs struct {
	Asset   string `json:"asset"`
	AgentID string `json:"agent_id"`
}

func (t *ListAssetLinks) Name() tool.Name                 { return ListAssetLinksName }
func (t *ListAssetLinks) InputSchema() *jsonschema.Schema { return mustSchema[assetRefArgs]() }
func (t *ListAssetLinks) Description() string {
	return "List the agent IDs linked to an asset. These links determine who receives and may invoke its operational tools."
}
func (t *ListAssetLinks) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(ListAssetLinksName))
	if err != nil {
		return nil, err
	}
	value, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("resolve asset: %w", err)
	}
	result, err := listAssetLinksResult(ctx, t.deps, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("list asset links: %w", err)
	}
	return json.Marshal(result)
}

func (t *LinkAsset) Name() tool.Name                 { return LinkAssetName }
func (t *LinkAsset) InputSchema() *jsonschema.Schema { return mustSchema[assetLinkArgs]() }
func (t *LinkAsset) Description() string {
	return "Link an asset to an agent, granting that agent the asset's operational MCP tools."
}
func (t *LinkAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetLinkArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(LinkAssetName))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.AgentID) == "" {
		return nil, errors.New("agent_id is required")
	}
	value, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("resolve asset: %w", err)
	}
	if _, err := t.deps.Assets.Link(ctx, orgID, value.ID, args.AgentID); err != nil {
		return nil, fmt.Errorf("link asset: %w", err)
	}
	result, err := listAssetLinksResult(ctx, t.deps, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("list asset links after link: %w", err)
	}
	return json.Marshal(result)
}

func (t *UnlinkAsset) Name() tool.Name                 { return UnlinkAssetName }
func (t *UnlinkAsset) InputSchema() *jsonschema.Schema { return mustSchema[assetLinkArgs]() }
func (t *UnlinkAsset) Description() string {
	return "Unlink an asset from an agent. Its derived operational tools are removed when no other linked asset still requires them."
}
func (t *UnlinkAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetLinkArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(UnlinkAssetName))
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.AgentID) == "" {
		return nil, errors.New("agent_id is required")
	}
	value, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("resolve asset: %w", err)
	}
	if err := t.deps.Assets.Unlink(ctx, orgID, value.ID, args.AgentID); err != nil {
		return nil, fmt.Errorf("unlink asset: %w", err)
	}
	result, err := listAssetLinksResult(ctx, t.deps, orgID, value)
	if err != nil {
		return nil, fmt.Errorf("list asset links after unlink: %w", err)
	}
	return json.Marshal(result)
}

// --- get_asset_health -----------------------------------------------------

const GetAssetHealthName tool.Name = "get_asset_health"

type GetAssetHealth struct{ deps Deps }

func (t *GetAssetHealth) Name() tool.Name                 { return GetAssetHealthName }
func (t *GetAssetHealth) InputSchema() *jsonschema.Schema { return mustSchema[assetRefArgs]() }
func (t *GetAssetHealth) Description() string {
	return "Check TCP and authenticated SSH reachability for any org server asset. Run this after installing a generated Helix public key, then exercise a server operation before claiming setup is complete."
}
func (t *GetAssetHealth) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, err := managedAssetsOrgID(inv, string(GetAssetHealthName))
	if err != nil {
		return nil, err
	}
	if t.deps.AssetHealth == nil {
		return nil, errors.New("asset health checker is not wired")
	}
	if _, err := resolveManagedAsset(t.deps, ctx, orgID, args.Asset); err != nil {
		return nil, fmt.Errorf("resolve asset: %w", err)
	}
	return json.Marshal(t.deps.AssetHealth(ctx, orgID, args.Asset))
}
