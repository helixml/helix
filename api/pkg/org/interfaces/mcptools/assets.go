package mcptools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/assetssh"
)

type ServerAssetRuntime interface {
	Run(ctx context.Context, orgID, agentID, assetRef string, req assetssh.RunRequest) (assetssh.Command, error)
	ListCommands(ctx context.Context, orgID, agentID, assetRef string) ([]assetssh.Command, error)
	GetCommand(ctx context.Context, orgID, agentID, assetRef, commandID string) (assetssh.Command, error)
	KillCommand(ctx context.Context, orgID, agentID, assetRef, commandID, signal string) error
	ListFiles(ctx context.Context, orgID, agentID, assetRef, directory string) ([]assetssh.FileEntry, error)
	ReadFile(ctx context.Context, orgID, agentID, assetRef, filename string, maxBytes int64) ([]byte, error)
	WriteFile(ctx context.Context, orgID, agentID, assetRef, filename string, data []byte, mode uint32) error
}

type AssetSSHIdentityIssuer interface {
	Mint(ctx context.Context, orgID, agentID, assetRef string) (assetssh.ProxyIdentity, error)
}

type assetView struct {
	ID             string           `json:"id"`
	Name           string           `json:"name"`
	Description    string           `json:"description,omitempty"`
	NotesForAgents string           `json:"notes_for_agents,omitempty"`
	Enabled        bool             `json:"enabled"`
	Kind           asset.Kind       `json:"kind"`
	Server         *assetServerView `json:"server,omitempty"`
}

type assetServerView struct {
	Address      string             `json:"address"`
	Port         uint16             `json:"port"`
	User         string             `json:"user"`
	AuthType     asset.AuthType     `json:"auth_type"`
	Capabilities []string           `json:"capabilities"`
	SSHAccess    assetSSHAccessView `json:"ssh_access"`
}

type assetSSHAccessView struct {
	Available    bool   `json:"available"`
	Tool         string `json:"tool"`
	Target       string `json:"target"`
	Instructions string `json:"instructions"`
}

func viewAsset(a asset.Asset) assetView {
	view := assetView{ID: a.ID, Name: a.Name, Description: a.Description, NotesForAgents: a.NotesForAgents, Enabled: !a.Disabled, Kind: a.Kind}
	if a.Config.Server != nil {
		capabilities := []string{
			"run_commands", "manage_detached_commands", "read_write_files", "ssh_via_helix_proxy",
		}
		sshAccess := assetSSHAccessView{
			Available: true, Tool: string(ServerSSHAccessName), Target: a.Name + "@<helix-ssh-proxy>",
			Instructions: "Call server_ssh_access to mint a short-lived identity and receive the exact SSH setup command.",
		}
		if a.Disabled {
			capabilities = nil
			sshAccess.Available = false
			sshAccess.Instructions = "This asset is disabled. An organization owner must enable it before agents can use MCP or SSH access."
		}
		view.Server = &assetServerView{
			Address: a.Config.Server.Address, Port: a.Config.Server.Port,
			User: a.Config.Server.User, AuthType: a.Config.Server.AuthType,
			Capabilities: capabilities,
			SSHAccess:    sshAccess,
		}
	}
	return view
}

func assetCaller(inv tool.Invocation, operation string) (orgID, agentID string, err error) {
	if inv.Caller == nil {
		return "", "", fmt.Errorf("%s: caller is missing", operation)
	}
	orgID, agentID = inv.Caller.OrganizationID(), inv.Caller.ID()
	if orgID == "" || agentID == "" {
		return "", "", fmt.Errorf("%s: caller has no organization or agent ID", operation)
	}
	return orgID, agentID, nil
}

type ListAssets struct{ deps Deps }

const ListAssetsName tool.Name = "list_assets"

func (t *ListAssets) Name() tool.Name                 { return ListAssetsName }
func (t *ListAssets) InputSchema() *jsonschema.Schema { return mustSchema[struct{}]() }
func (t *ListAssets) Description() string {
	return "List server assets linked to this agent, including enabled or disabled status, connection coordinates, operator notes, command/file capabilities, and SSH proxy guidance."
}
func (t *ListAssets) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	orgID, agentID, err := assetCaller(inv, ListAssetsName)
	if err != nil {
		return nil, err
	}
	if t.deps.Assets == nil {
		return nil, errors.New("assets service is not wired")
	}
	all, err := t.deps.Assets.ListForAgent(ctx, orgID, agentID)
	if err != nil {
		return nil, fmt.Errorf("list linked assets: %w", err)
	}
	views := make([]assetView, 0, len(all))
	for _, a := range all {
		views = append(views, viewAsset(a))
	}
	return json.Marshal(map[string]any{"assets": views})
}

type assetRefArgs struct {
	Asset string `json:"asset"`
}

type GetAsset struct{ deps Deps }

const GetAssetName tool.Name = "get_asset"

func (t *GetAsset) Name() tool.Name                 { return GetAssetName }
func (t *GetAsset) InputSchema() *jsonschema.Schema { return mustSchema[assetRefArgs]() }
func (t *GetAsset) Description() string {
	return "Get a linked server asset by ID or name, including its notes, command/file capabilities, and SSH proxy guidance."
}
func (t *GetAsset) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, GetAssetName)
	if err != nil {
		return nil, err
	}
	if t.deps.Assets == nil {
		return nil, errors.New("assets service is not wired")
	}
	a, err := t.deps.Assets.AuthorizeRef(ctx, orgID, agentID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("get linked asset: %w", err)
	}
	return json.Marshal(viewAsset(a))
}

type serverRunArgs struct {
	Asset          string            `json:"asset"`
	Command        string            `json:"command"`
	Args           []string          `json:"args,omitempty"`
	Cwd            string            `json:"cwd,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Sudo           bool              `json:"sudo,omitempty"`
	Detached       bool              `json:"detached,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
}

type ServerRunCommand struct{ deps Deps }

const ServerRunCommandName tool.Name = "server_run_command"

func (t *ServerRunCommand) Name() tool.Name                 { return ServerRunCommandName }
func (t *ServerRunCommand) InputSchema() *jsonschema.Schema { return mustSchema[serverRunArgs]() }
func (t *ServerRunCommand) Description() string {
	return "Run an argv-safe command on a linked server. Set detached=true for a background command, then inspect it with server_get_command."
}
func (t *ServerRunCommand) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverRunArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerRunCommandName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	command, err := t.deps.AssetSSH.Run(ctx, orgID, agentID, args.Asset, assetssh.RunRequest{
		Cmd: args.Command, Args: args.Args, Cwd: args.Cwd, Env: args.Env, Sudo: args.Sudo,
		Detached: args.Detached, TimeoutSeconds: args.TimeoutSeconds,
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(command)
}

type serverCommandsArgs struct {
	Asset string `json:"asset"`
}

type ServerListCommands struct{ deps Deps }

const ServerListCommandsName tool.Name = "server_list_commands"

func (t *ServerListCommands) Name() tool.Name { return ServerListCommandsName }
func (t *ServerListCommands) InputSchema() *jsonschema.Schema {
	return mustSchema[serverCommandsArgs]()
}
func (t *ServerListCommands) Description() string { return "List commands started on a linked server." }
func (t *ServerListCommands) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverCommandsArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerListCommandsName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	commands, err := t.deps.AssetSSH.ListCommands(ctx, orgID, agentID, args.Asset)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"commands": commands})
}

type serverCommandArgs struct {
	Asset   string `json:"asset"`
	Command string `json:"command"`
}

type ServerGetCommand struct{ deps Deps }

const ServerGetCommandName tool.Name = "server_get_command"

func (t *ServerGetCommand) Name() tool.Name                 { return ServerGetCommandName }
func (t *ServerGetCommand) InputSchema() *jsonschema.Schema { return mustSchema[serverCommandArgs]() }
func (t *ServerGetCommand) Description() string {
	return "Get one command's status, exit code, stdout, and stderr."
}
func (t *ServerGetCommand) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverCommandArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerGetCommandName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	command, err := t.deps.AssetSSH.GetCommand(ctx, orgID, agentID, args.Asset, args.Command)
	if err != nil {
		return nil, err
	}
	return json.Marshal(command)
}

type serverKillArgs struct {
	Asset   string `json:"asset"`
	Command string `json:"command"`
	Signal  string `json:"signal,omitempty"`
}

type ServerKillCommand struct{ deps Deps }

const ServerKillCommandName tool.Name = "server_kill_command"

func (t *ServerKillCommand) Name() tool.Name                 { return ServerKillCommandName }
func (t *ServerKillCommand) InputSchema() *jsonschema.Schema { return mustSchema[serverKillArgs]() }
func (t *ServerKillCommand) Description() string {
	return "Signal a running detached server command. Supported signals: TERM, KILL, INT, HUP."
}
func (t *ServerKillCommand) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverKillArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerKillCommandName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	if err := t.deps.AssetSSH.KillCommand(ctx, orgID, agentID, args.Asset, args.Command, args.Signal); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"command": args.Command, "status": "signal_sent"})
}

type serverListFilesArgs struct {
	Asset string `json:"asset"`
	Path  string `json:"path"`
}

type ServerListFiles struct{ deps Deps }

const ServerListFilesName tool.Name = "server_list_files"

func (t *ServerListFiles) Name() tool.Name { return ServerListFilesName }
func (t *ServerListFiles) InputSchema() *jsonschema.Schema {
	return mustSchema[serverListFilesArgs]()
}
func (t *ServerListFiles) Description() string {
	return "List an absolute directory on a linked server."
}
func (t *ServerListFiles) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverListFilesArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerListFilesName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	entries, err := t.deps.AssetSSH.ListFiles(ctx, orgID, agentID, args.Asset, args.Path)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"files": entries})
}

type serverReadFileArgs struct {
	Asset    string `json:"asset"`
	Path     string `json:"path"`
	Encoding string `json:"encoding,omitempty"`
	MaxBytes int64  `json:"max_bytes,omitempty"`
}

type ServerReadFile struct{ deps Deps }

const ServerReadFileName tool.Name = "server_read_file"

func (t *ServerReadFile) Name() tool.Name                 { return ServerReadFileName }
func (t *ServerReadFile) InputSchema() *jsonschema.Schema { return mustSchema[serverReadFileArgs]() }
func (t *ServerReadFile) Description() string {
	return "Read an absolute file from a linked server as utf8 (default) or base64; defaults to a 1 MiB limit."
}
func (t *ServerReadFile) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverReadFileArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerReadFileName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	data, err := t.deps.AssetSSH.ReadFile(ctx, orgID, agentID, args.Asset, args.Path, args.MaxBytes)
	if err != nil {
		return nil, err
	}
	encoding := strings.ToLower(strings.TrimSpace(args.Encoding))
	if encoding == "" {
		encoding = "utf8"
	}
	var content string
	switch encoding {
	case "utf8":
		content = string(data)
	case "base64":
		content = base64.StdEncoding.EncodeToString(data)
	default:
		return nil, fmt.Errorf("unsupported encoding %q", args.Encoding)
	}
	return json.Marshal(map[string]any{"path": args.Path, "encoding": encoding, "content": content, "size": len(data)})
}

type serverWriteFileArgs struct {
	Asset    string `json:"asset"`
	Path     string `json:"path"`
	Content  string `json:"content"`
	Encoding string `json:"encoding,omitempty"`
	Mode     uint32 `json:"mode,omitempty"`
}

type ServerWriteFile struct{ deps Deps }

const ServerWriteFileName tool.Name = "server_write_file"

func (t *ServerWriteFile) Name() tool.Name { return ServerWriteFileName }
func (t *ServerWriteFile) InputSchema() *jsonschema.Schema {
	return mustSchema[serverWriteFileArgs]()
}

type ServerSSHAccess struct{ deps Deps }

const ServerSSHAccessName tool.Name = "server_ssh_access"

func (t *ServerSSHAccess) Name() tool.Name { return ServerSSHAccessName }
func (t *ServerSSHAccess) InputSchema() *jsonschema.Schema {
	return mustSchema[assetRefArgs]()
}
func (t *ServerSSHAccess) Description() string {
	return "Mint a one-hour SSH certificate for this agent to a linked server and return the exact setup command. After setup, use normal ssh <asset-name>@<Helix-proxy>."
}
func (t *ServerSSHAccess) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args assetRefArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerSSHAccessName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSHIssuer == nil || t.deps.AssetSSHProxyAddress == "" {
		return nil, errors.New("asset SSH proxy is not configured")
	}
	identity, err := t.deps.AssetSSHIssuer.Mint(ctx, orgID, agentID, args.Asset)
	if err != nil {
		return nil, fmt.Errorf("mint asset SSH access: %w", err)
	}
	host, port, err := assetssh.ParseProxyAddress(t.deps.AssetSSHProxyAddress)
	if err != nil {
		return nil, err
	}
	identityPath := "$HOME/.ssh/helix-assets-" + identity.AssetID
	usage := fmt.Sprintf(
		"install -d -m 700 \"$HOME/.ssh\" && printf '%%s' '%s' | base64 -d > \"%s\" && printf '%%s' '%s' | base64 -d > \"%s-cert.pub\" && chmod 600 \"%s\" \"%s-cert.pub\"; ssh -i \"%s\" -p %d %s@%s",
		base64.StdEncoding.EncodeToString([]byte(identity.PrivateKey)), identityPath,
		base64.StdEncoding.EncodeToString([]byte(identity.Certificate+"\n")), identityPath,
		identityPath, identityPath, identityPath, port, identity.AssetName, host,
	)
	return json.Marshal(map[string]any{
		"asset":       identity.AssetName,
		"proxy_host":  host,
		"proxy_port":  port,
		"private_key": identity.PrivateKey,
		"certificate": identity.Certificate,
		"expires_at":  identity.ExpiresAt,
		"usage":       usage,
	})
}
func (t *ServerWriteFile) Description() string {
	return "Write or replace an absolute file on a linked server from utf8 (default) or base64 content, creating parent directories."
}
func (t *ServerWriteFile) Invoke(ctx context.Context, inv tool.Invocation) (json.RawMessage, error) {
	var args serverWriteFileArgs
	if err := json.Unmarshal(inv.Args, &args); err != nil {
		return nil, fmt.Errorf("parse args: %w", err)
	}
	orgID, agentID, err := assetCaller(inv, ServerWriteFileName)
	if err != nil {
		return nil, err
	}
	if t.deps.AssetSSH == nil {
		return nil, errors.New("asset SSH runtime is not wired")
	}
	encoding := strings.ToLower(strings.TrimSpace(args.Encoding))
	if encoding == "" {
		encoding = "utf8"
	}
	var data []byte
	switch encoding {
	case "utf8":
		data = []byte(args.Content)
	case "base64":
		data, err = base64.StdEncoding.DecodeString(args.Content)
		if err != nil {
			return nil, fmt.Errorf("decode base64 content: %w", err)
		}
	default:
		return nil, fmt.Errorf("unsupported encoding %q", args.Encoding)
	}
	if err := t.deps.AssetSSH.WriteFile(ctx, orgID, agentID, args.Asset, args.Path, data, args.Mode); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"path": args.Path, "bytes_written": len(data)})
}
