package assets

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
)

var ServerTools = []tool.Name{
	"list_assets",
	"get_asset",
	"server_run_command",
	"server_list_commands",
	"server_get_command",
	"server_kill_command",
	"server_list_files",
	"server_read_file",
	"server_write_file",
	"server_ssh_access",
}

type KeyGenerator func() (privateKeyPEM, publicKeyOpenSSH string, err error)
type Encrypt func(plaintext []byte) (string, error)

type Service struct {
	assets            store.Assets
	links             store.AssetLinks
	nodes             store.Nodes
	generateKey       KeyGenerator
	encrypt           Encrypt
	now               func() time.Time
	newID             func() string
	onToolsChanged    func(context.Context, string)
	onRestartRequired func(context.Context, string, orgchart.NodeID)
}

type Deps struct {
	Assets         store.Assets
	Links          store.AssetLinks
	Nodes          store.Nodes
	GenerateKey    KeyGenerator
	Encrypt        Encrypt
	Now            func() time.Time
	NewID          func() string
	OnToolsChanged func(context.Context, string)
	// OnRestartRequired fires after Link/Unlink changes a Node's restart
	// fingerprint (its granted tool list), mirroring the same signal
	// nodes.Nodes emits for direct tool edits. nil disables it.
	OnRestartRequired func(context.Context, string, orgchart.NodeID)
}

func New(deps Deps) (*Service, error) {
	if deps.Assets == nil || deps.Links == nil || deps.Nodes == nil {
		return nil, errors.New("assets service requires assets, links, and nodes stores")
	}
	if deps.GenerateKey == nil || deps.Encrypt == nil {
		return nil, errors.New("assets service requires SSH key generation and encryption")
	}
	now := deps.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	newID := deps.NewID
	if newID == nil {
		return nil, errors.New("assets service requires an ID generator")
	}
	return &Service{
		assets: deps.Assets, links: deps.Links, nodes: deps.Nodes,
		generateKey: deps.GenerateKey, encrypt: deps.Encrypt, now: now, newID: newID,
		onToolsChanged:    deps.OnToolsChanged,
		onRestartRequired: deps.OnRestartRequired,
	}, nil
}

type CreateServerParams struct {
	ID             string
	Name           string
	Description    string
	NotesForAgents string
	Address        string
	Port           uint16
	User           string
	AuthType       asset.AuthType
	Password       string
	HostKey        string
}

func (s *Service) CreateServer(ctx context.Context, orgID string, p CreateServerParams) (asset.Asset, error) {
	authType := p.AuthType
	if authType == "" {
		authType = asset.AuthSSHKey
	}
	var publicKey, encryptedPrivateKey, encryptedPassword string
	if authType == asset.AuthSSHKey {
		privateKey, generatedPublicKey, err := s.generateKey()
		if err != nil {
			return asset.Asset{}, fmt.Errorf("generate server SSH key: %w", err)
		}
		publicKey = generatedPublicKey
		encryptedPrivateKey, err = s.encrypt([]byte(privateKey))
		if err != nil {
			return asset.Asset{}, fmt.Errorf("encrypt server SSH private key: %w", err)
		}
	} else if authType == asset.AuthPassword {
		if p.Password == "" {
			return asset.Asset{}, errors.New("server password is empty")
		}
		var err error
		encryptedPassword, err = s.encrypt([]byte(p.Password))
		if err != nil {
			return asset.Asset{}, fmt.Errorf("encrypt server SSH password: %w", err)
		}
	}
	port := p.Port
	if port == 0 {
		port = 22
	}
	id := strings.TrimSpace(p.ID)
	if id == "" {
		id = "a-" + s.newID()
	}
	a, err := asset.New(id, orgID, p.Name, p.Description, p.NotesForAgents, asset.KindServer, asset.Config{
		Server: &asset.Server{
			Address: strings.TrimSpace(p.Address), Port: port, User: strings.TrimSpace(p.User),
			AuthType: authType, PublicKey: strings.TrimSpace(publicKey), EncryptedPrivateKey: encryptedPrivateKey,
			EncryptedPassword: encryptedPassword,
			HostKey:           strings.TrimSpace(p.HostKey),
		},
	}, s.now())
	if err != nil {
		return asset.Asset{}, err
	}
	if err := s.assets.Create(ctx, a); err != nil {
		return asset.Asset{}, err
	}
	return a, nil
}

type UpdateServerParams struct {
	Name           *string
	Description    *string
	NotesForAgents *string
	Enabled        *bool
	Address        *string
	Port           *uint16
	User           *string
	AuthType       *asset.AuthType
	Password       *string
	HostKey        *string
}

func (s *Service) UpdateServer(ctx context.Context, orgID string, id asset.ID, p UpdateServerParams) (asset.Asset, error) {
	a, err := s.assets.Get(ctx, orgID, id)
	if err != nil {
		return asset.Asset{}, err
	}
	if a.Kind != asset.KindServer || a.Config.Server == nil {
		return asset.Asset{}, fmt.Errorf("asset %q is not a server", id)
	}
	server := *a.Config.Server
	endpointChanged := false
	if p.Name != nil {
		a.Name = strings.TrimSpace(*p.Name)
	}
	if p.Description != nil {
		a.Description = strings.TrimSpace(*p.Description)
	}
	if p.NotesForAgents != nil {
		a.NotesForAgents = strings.TrimSpace(*p.NotesForAgents)
	}
	if p.Enabled != nil {
		a.Disabled = !*p.Enabled
	}
	if p.Address != nil {
		address := strings.TrimSpace(*p.Address)
		endpointChanged = address != server.Address
		server.Address = address
	}
	if p.Port != nil {
		endpointChanged = endpointChanged || *p.Port != server.Port
		server.Port = *p.Port
	}
	if p.User != nil {
		server.User = strings.TrimSpace(*p.User)
	}
	if p.AuthType != nil && *p.AuthType != server.AuthType {
		switch *p.AuthType {
		case asset.AuthSSHKey:
			privateKey, publicKey, err := s.generateKey()
			if err != nil {
				return asset.Asset{}, fmt.Errorf("generate server SSH key: %w", err)
			}
			encryptedPrivateKey, err := s.encrypt([]byte(privateKey))
			if err != nil {
				return asset.Asset{}, fmt.Errorf("encrypt server SSH private key: %w", err)
			}
			server.AuthType = asset.AuthSSHKey
			server.PublicKey = strings.TrimSpace(publicKey)
			server.EncryptedPrivateKey = encryptedPrivateKey
			server.EncryptedPassword = ""
		case asset.AuthPassword:
			if p.Password == nil || *p.Password == "" {
				return asset.Asset{}, errors.New("server password is required when switching to password authentication")
			}
			encryptedPassword, err := s.encrypt([]byte(*p.Password))
			if err != nil {
				return asset.Asset{}, fmt.Errorf("encrypt server SSH password: %w", err)
			}
			server.AuthType = asset.AuthPassword
			server.EncryptedPassword = encryptedPassword
			server.PublicKey = ""
			server.EncryptedPrivateKey = ""
		default:
			return asset.Asset{}, fmt.Errorf("unsupported server auth type %q", *p.AuthType)
		}
	} else if p.Password != nil {
		if server.AuthType != asset.AuthPassword {
			return asset.Asset{}, errors.New("password can only be updated for password authentication")
		}
		if *p.Password == "" {
			return asset.Asset{}, errors.New("server password is empty")
		}
		encryptedPassword, err := s.encrypt([]byte(*p.Password))
		if err != nil {
			return asset.Asset{}, fmt.Errorf("encrypt server SSH password: %w", err)
		}
		server.EncryptedPassword = encryptedPassword
	}
	if p.HostKey != nil {
		server.HostKey = strings.TrimSpace(*p.HostKey)
	} else if endpointChanged {
		server.HostKey = ""
	}
	a.Config.Server = &server
	a.UpdatedAt = s.now()
	if err := a.Validate(); err != nil {
		return asset.Asset{}, err
	}
	if err := s.assets.Update(ctx, a); err != nil {
		return asset.Asset{}, err
	}
	return a, nil
}

func (s *Service) Get(ctx context.Context, orgID string, id asset.ID) (asset.Asset, error) {
	return s.assets.Get(ctx, orgID, id)
}

func (s *Service) GetByName(ctx context.Context, orgID, name string) (asset.Asset, error) {
	return s.assets.GetByName(ctx, orgID, name)
}

func (s *Service) Resolve(ctx context.Context, orgID, idOrName string) (asset.Asset, error) {
	a, err := s.assets.Get(ctx, orgID, idOrName)
	if err == nil {
		return a, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return asset.Asset{}, err
	}
	return s.assets.GetByName(ctx, orgID, idOrName)
}

func (s *Service) List(ctx context.Context, orgID string) ([]asset.Asset, error) {
	return s.assets.List(ctx, orgID)
}

func (s *Service) ListForAgent(ctx context.Context, orgID, agentID string) ([]asset.Asset, error) {
	links, err := s.links.ListForAgent(ctx, orgID, agentID)
	if err != nil {
		return nil, err
	}
	out := make([]asset.Asset, 0, len(links))
	for _, link := range links {
		a, err := s.assets.Get(ctx, orgID, link.AssetID)
		if err != nil {
			return nil, fmt.Errorf("get linked asset %q: %w", link.AssetID, err)
		}
		out = append(out, a)
	}
	return out, nil
}

func (s *Service) ListLinks(ctx context.Context, orgID string, assetID asset.ID) ([]asset.Link, error) {
	if _, err := s.assets.Get(ctx, orgID, assetID); err != nil {
		return nil, err
	}
	return s.links.ListForAsset(ctx, orgID, assetID)
}

func (s *Service) Link(ctx context.Context, orgID string, assetID asset.ID, agentID string) (asset.Link, error) {
	a, err := s.assets.Get(ctx, orgID, assetID)
	if err != nil {
		return asset.Link{}, err
	}
	agent, err := s.nodes.Get(ctx, orgID, agentID)
	if err != nil {
		return asset.Link{}, err
	}
	link, err := asset.NewLink(orgID, assetID, agentID, s.now())
	if err != nil {
		return asset.Link{}, err
	}
	if err := s.links.Create(ctx, link); err != nil {
		return asset.Link{}, err
	}
	before := agent
	agent = agent.WithTools(nodes.MergeTools(agent.Tools, toolsForKind(a.Kind))).WithUpdatedAt(s.now())
	if err := s.nodes.Update(ctx, agent); err != nil {
		_ = s.links.Delete(ctx, orgID, assetID, agentID)
		return asset.Link{}, fmt.Errorf("attach asset tools to agent: %w", err)
	}
	s.notifyToolsChanged(ctx, agent.AgentID)
	s.notifyRestartRequired(ctx, orgID, before, agent)
	return link, nil
}

func (s *Service) Unlink(ctx context.Context, orgID string, assetID asset.ID, agentID string) error {
	a, err := s.assets.Get(ctx, orgID, assetID)
	if err != nil {
		return err
	}
	agent, err := s.nodes.Get(ctx, orgID, agentID)
	if err != nil {
		return err
	}
	link, err := s.links.Find(ctx, orgID, assetID, agentID)
	if err != nil {
		return err
	}
	if err := s.links.Delete(ctx, orgID, assetID, agentID); err != nil {
		return err
	}
	keep, err := s.toolsRequiredByOtherLinks(ctx, orgID, agentID)
	if err != nil {
		_ = s.links.Create(ctx, link)
		return err
	}
	remove := toolsForKind(a.Kind)
	tools := make([]tool.Name, 0, len(agent.Tools))
	for _, name := range agent.Tools {
		if slices.Contains(remove, name) && !keep[name] {
			continue
		}
		tools = append(tools, name)
	}
	before := agent
	agent = agent.WithTools(tools).WithUpdatedAt(s.now())
	if err := s.nodes.Update(ctx, agent); err != nil {
		_ = s.links.Create(ctx, link)
		return fmt.Errorf("detach asset tools from agent: %w", err)
	}
	s.notifyToolsChanged(ctx, agent.AgentID)
	s.notifyRestartRequired(ctx, orgID, before, agent)
	return nil
}

func (s *Service) notifyToolsChanged(ctx context.Context, appID string) {
	if s.onToolsChanged != nil && appID != "" {
		s.onToolsChanged(ctx, appID)
	}
}

// notifyRestartRequired fires OnRestartRequired when Link/Unlink changed
// the agent's restart fingerprint, mirroring nodes.Nodes's own
// before/after check so a no-op re-link (tools already present/absent)
// does not nag the operator.
func (s *Service) notifyRestartRequired(ctx context.Context, orgID string, before, after orgchart.Node) {
	if s.onRestartRequired == nil {
		return
	}
	if orgchart.RestartFingerprint(before) == orgchart.RestartFingerprint(after) {
		return
	}
	s.onRestartRequired(ctx, orgID, after.ID)
}

func (s *Service) Delete(ctx context.Context, orgID string, id asset.ID) error {
	links, err := s.links.ListForAsset(ctx, orgID, id)
	if err != nil {
		return err
	}
	for _, link := range links {
		if err := s.Unlink(ctx, orgID, id, link.AgentID); err != nil {
			return fmt.Errorf("unlink asset from agent %q: %w", link.AgentID, err)
		}
	}
	return s.assets.Delete(ctx, orgID, id)
}

func (s *Service) Authorize(ctx context.Context, orgID, agentID string, assetID asset.ID) (asset.Asset, error) {
	if _, err := s.links.Find(ctx, orgID, assetID, agentID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return asset.Asset{}, fmt.Errorf("agent %q is not linked to asset %q", agentID, assetID)
		}
		return asset.Asset{}, err
	}
	a, err := s.assets.Get(ctx, orgID, assetID)
	if err != nil {
		return asset.Asset{}, err
	}
	if a.Disabled {
		return asset.Asset{}, fmt.Errorf("asset %q is disabled", a.Name)
	}
	return a, nil
}

func (s *Service) AuthorizeRef(ctx context.Context, orgID, agentID, idOrName string) (asset.Asset, error) {
	a, err := s.Resolve(ctx, orgID, idOrName)
	if err != nil {
		return asset.Asset{}, err
	}
	return s.Authorize(ctx, orgID, agentID, a.ID)
}

func (s *Service) PinHostKey(ctx context.Context, orgID string, id asset.ID, hostKey string) error {
	a, err := s.assets.Get(ctx, orgID, id)
	if err != nil {
		return err
	}
	if a.Kind != asset.KindServer || a.Config.Server == nil {
		return fmt.Errorf("asset %q is not a server", id)
	}
	pinned := strings.TrimSpace(a.Config.Server.HostKey)
	hostKey = strings.TrimSpace(hostKey)
	if pinned != "" && pinned != hostKey {
		return errors.New("server SSH host key changed")
	}
	if pinned == hostKey {
		return nil
	}
	server := *a.Config.Server
	server.HostKey = hostKey
	a.Config.Server = &server
	a.UpdatedAt = s.now()
	return s.assets.Update(ctx, a)
}

func (s *Service) toolsRequiredByOtherLinks(ctx context.Context, orgID, agentID string) (map[tool.Name]bool, error) {
	links, err := s.links.ListForAgent(ctx, orgID, agentID)
	if err != nil {
		return nil, err
	}
	keep := make(map[tool.Name]bool)
	for _, link := range links {
		a, err := s.assets.Get(ctx, orgID, link.AssetID)
		if err != nil {
			return nil, err
		}
		for _, name := range toolsForKind(a.Kind) {
			keep[name] = true
		}
	}
	return keep, nil
}

func toolsForKind(kind asset.Kind) []tool.Name {
	if kind == asset.KindServer {
		return append([]tool.Name(nil), ServerTools...)
	}
	return nil
}
