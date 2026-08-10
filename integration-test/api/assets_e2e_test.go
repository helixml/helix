package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/helixml/helix/api/pkg/auth"
	"github.com/helixml/helix/api/pkg/config"
	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	"github.com/helixml/helix/api/pkg/org/infrastructure/assetssh"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"
)

func TestServerAssetE2E(t *testing.T) {
	if os.Getenv("START_HELIX_TEST_SERVER") != "true" {
		t.Skip("set START_HELIX_TEST_SERVER=true to run API integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := getStoreClient()
	require.NoError(t, err)
	authenticator, err := auth.NewHelixAuthenticator(&config.ServerConfig{}, db, "test-secret", nil)
	require.NoError(t, err)

	owner, ownerKey := createAssetE2EUser(t, db, authenticator, "owner")
	member, memberKey := createAssetE2EUser(t, db, authenticator, "member")
	_, outsiderKey := createAssetE2EUser(t, db, authenticator, "outsider")
	ownerClient, err := getAPIClient(ownerKey)
	require.NoError(t, err)
	organization, err := ownerClient.CreateOrganization(ctx, &types.Organization{Name: "asset-e2e-" + uuid.NewString()})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ownerClient.DeleteOrganization(context.Background(), organization.ID)) })
	_, err = ownerClient.AddOrganizationMember(ctx, organization.ID, &types.AddOrganizationMemberRequest{
		UserReference: member.ID,
		Role:          types.OrganizationRoleMember,
	})
	require.NoError(t, err)

	var agents []orgapi.BotDTO
	assetE2ERequest(t, ownerKey, http.MethodGet, "/api/v1/orgs/"+organization.Name+"/agents", nil, http.StatusOK, &agents)
	require.Contains(t, agentIDs(agents), "chief-of-staff")

	sshServer := startAssetE2ESSHServer(t, "ubuntu")
	host, rawPort, err := net.SplitHostPort(sshServer.address())
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)

	createRequest := orgapi.CreateAssetRequest{
		Name: "ci-server", Description: "CI asset server", NotesForAgents: "Use only the integration-test workspace.",
		Kind: asset.KindServer,
		Server: &orgapi.ServerAssetWriteRequest{
			Address: host, Port: uint16(port), User: "ubuntu", AuthType: asset.AuthSSHKey,
		},
	}
	var created orgapi.AssetDTO
	assetE2ERequest(t, ownerKey, http.MethodPost, assetCollectionPath(organization.Name), createRequest, http.StatusCreated, &created)
	require.NotEmpty(t, created.Server.PublicKey)
	sshServer.authorize(t, created.Server.PublicKey)

	var listed orgapi.AssetsResponse
	assetE2ERequest(t, memberKey, http.MethodGet, assetCollectionPath(organization.Name), nil, http.StatusOK, &listed)
	require.Len(t, listed.Assets, 1)
	assetE2ERequest(t, memberKey, http.MethodGet, assetItemPath(organization.Name, created.ID), nil, http.StatusOK, nil)
	assetE2ERequest(t, memberKey, http.MethodPatch, assetItemPath(organization.Name, created.ID), map[string]string{"description": "forbidden"}, http.StatusForbidden, nil)
	assetE2ERequest(t, outsiderKey, http.MethodGet, assetCollectionPath(organization.Name), nil, http.StatusForbidden, nil)
	assetE2ERequest(t, memberKey, http.MethodPost, assetCollectionPath(organization.Name), createRequest, http.StatusForbidden, nil)

	var health orgapi.AssetHealthDTO
	assetE2ERequest(t, ownerKey, http.MethodGet, assetItemPath(organization.Name, created.ID)+"/health", nil, http.StatusOK, &health)
	require.True(t, health.TCPReachable)
	require.True(t, health.SSHReachable, health.Error)

	description := "Updated without losing server credentials"
	notes := "Visible to the linked agent through MCP."
	var updated orgapi.AssetDTO
	assetE2ERequest(t, ownerKey, http.MethodPatch, assetItemPath(organization.Name, created.ID), orgapi.UpdateAssetRequest{
		Description: &description, NotesForAgents: &notes,
	}, http.StatusOK, &updated)
	require.Equal(t, host, updated.Server.Address)
	require.Equal(t, uint16(port), updated.Server.Port)
	require.NotEmpty(t, updated.Server.HostKeyFingerprint)

	assetE2ERequest(t, ownerKey, http.MethodPost, assetItemPath(organization.Name, created.ID)+"/links", orgapi.AssetLinkRequest{
		AgentID: "chief-of-staff",
	}, http.StatusCreated, nil)

	sessionKey := createAssetE2ESessionKey(t, db, owner, organization.ID, "chief-of-staff")
	mcpSession := connectAssetE2EMCP(t, sessionKey)
	tools, err := mcpSession.ListTools(ctx, nil)
	require.NoError(t, err)
	for _, name := range []string{
		"list_assets", "get_asset", "server_run_command", "server_list_commands", "server_get_command",
		"server_kill_command", "server_list_files", "server_read_file", "server_write_file", "server_ssh_access",
	} {
		require.Contains(t, mcpToolNames(tools.Tools), name)
	}

	var discovered struct {
		Assets []struct {
			Name           string `json:"name"`
			Description    string `json:"description"`
			NotesForAgents string `json:"notes_for_agents"`
			Server         struct {
				Capabilities []string `json:"capabilities"`
				SSHAccess    struct {
					Available bool   `json:"available"`
					Tool      string `json:"tool"`
				} `json:"ssh_access"`
			} `json:"server"`
		} `json:"assets"`
	}
	callAssetE2ETool(t, mcpSession, "list_assets", map[string]any{}, &discovered)
	require.Len(t, discovered.Assets, 1)
	require.Equal(t, description, discovered.Assets[0].Description)
	require.Equal(t, notes, discovered.Assets[0].NotesForAgents)
	require.ElementsMatch(t, []string{"run_commands", "manage_detached_commands", "read_write_files", "ssh_via_helix_proxy"}, discovered.Assets[0].Server.Capabilities)
	require.True(t, discovered.Assets[0].Server.SSHAccess.Available)
	require.Equal(t, "server_ssh_access", discovered.Assets[0].Server.SSHAccess.Tool)

	var foreground assetssh.Command
	callAssetE2ETool(t, mcpSession, "server_run_command", map[string]any{
		"asset": created.Name, "command": "/usr/bin/printf", "args": []string{"asset-command-ok"},
	}, &foreground)
	require.Equal(t, assetssh.CommandFinished, foreground.Status)
	require.Equal(t, "asset-command-ok", foreground.Stdout)
	require.NotNil(t, foreground.ExitCode)
	require.Zero(t, *foreground.ExitCode)

	var detached assetssh.Command
	callAssetE2ETool(t, mcpSession, "server_run_command", map[string]any{
		"asset": created.Name, "command": "/bin/sh", "args": []string{"-c", "sleep 30"}, "detached": true,
	}, &detached)
	require.Equal(t, assetssh.CommandRunning, detached.Status)
	var commands struct {
		Commands []assetssh.Command `json:"commands"`
	}
	callAssetE2ETool(t, mcpSession, "server_list_commands", map[string]any{"asset": created.Name}, &commands)
	require.Contains(t, commandIDs(commands.Commands), detached.ID)
	callAssetE2ETool(t, mcpSession, "server_kill_command", map[string]any{
		"asset": created.Name, "command": detached.ID, "signal": "KILL",
	}, nil)
	require.Eventually(t, func() bool {
		var command assetssh.Command
		callAssetE2ETool(t, mcpSession, "server_get_command", map[string]any{"asset": created.Name, "command": detached.ID}, &command)
		return command.Status == assetssh.CommandKilled
	}, 5*time.Second, 100*time.Millisecond)

	filename := t.TempDir() + "/nested/asset-e2e.txt"
	callAssetE2ETool(t, mcpSession, "server_write_file", map[string]any{
		"asset": created.Name, "path": filename, "content": "asset-file-ok", "mode": 0o640,
	}, nil)
	var file struct {
		Content string `json:"content"`
	}
	callAssetE2ETool(t, mcpSession, "server_read_file", map[string]any{"asset": created.Name, "path": filename}, &file)
	require.Equal(t, "asset-file-ok", file.Content)
	var files struct {
		Files []assetssh.FileEntry `json:"files"`
	}
	callAssetE2ETool(t, mcpSession, "server_list_files", map[string]any{"asset": created.Name, "path": pathDir(filename)}, &files)
	require.Contains(t, fileNames(files.Files), "asset-e2e.txt")

	var access struct {
		ProxyHost   string `json:"proxy_host"`
		ProxyPort   int    `json:"proxy_port"`
		PrivateKey  string `json:"private_key"`
		Certificate string `json:"certificate"`
	}
	callAssetE2ETool(t, mcpSession, "server_ssh_access", map[string]any{"asset": created.Name}, &access)
	proxyConfig := assetProxyClientConfig(t, created.Name, access.PrivateKey, access.Certificate)
	proxyClient, err := ssh.Dial("tcp", net.JoinHostPort(access.ProxyHost, strconv.Itoa(access.ProxyPort)), proxyConfig)
	require.NoError(t, err)
	proxySession, err := proxyClient.NewSession()
	require.NoError(t, err)
	output, err := proxySession.CombinedOutput("printf asset-proxy-ok")
	require.NoError(t, err)
	require.Equal(t, "asset-proxy-ok", string(output))
	require.NoError(t, proxyClient.Close())

	assetE2ERequest(t, ownerKey, http.MethodDelete, assetItemPath(organization.Name, created.ID)+"/links/chief-of-staff", nil, http.StatusNoContent, nil)
	revokedClient, err := ssh.Dial("tcp", net.JoinHostPort(access.ProxyHost, strconv.Itoa(access.ProxyPort)), proxyConfig)
	if err == nil {
		_ = revokedClient.Close()
	}
	require.Error(t, err, "previously minted identity must stop working after unlink")

	assetE2ERequest(t, ownerKey, http.MethodDelete, assetItemPath(organization.Name, created.ID), nil, http.StatusNoContent, nil)
	var recreated orgapi.AssetDTO
	assetE2ERequest(t, ownerKey, http.MethodPost, assetCollectionPath(organization.Name), createRequest, http.StatusCreated, &recreated)
	require.NotEqual(t, created.ID, recreated.ID)
}

func createAssetE2EUser(t *testing.T, db *store.PostgresStore, authenticator auth.Authenticator, role string) (*types.User, string) {
	t.Helper()
	user, apiKey, err := createUser(t, db, authenticator, fmt.Sprintf("asset-e2e-%s-%s@example.com", role, uuid.NewString()))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.DeleteUser(context.Background(), user.ID)) })
	user.AlphaFeatures = []string{"helix-org"}
	user, err = db.UpdateUser(context.Background(), user)
	require.NoError(t, err)
	return user, apiKey
}

func createAssetE2ESessionKey(t *testing.T, db *store.PostgresStore, owner *types.User, orgID, workerID string) string {
	t.Helper()
	ctx := context.Background()
	session, err := db.CreateSession(ctx, types.Session{
		ID:             system.GenerateSessionID(),
		Owner:          owner.ID,
		OwnerType:      types.OwnerTypeUser,
		OrganizationID: orgID,
		Metadata:       types.SessionMetadata{OrgWorkerID: workerID},
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := db.DeleteSession(context.Background(), session.ID)
		require.NoError(t, err)
	})

	key, err := system.GenerateAPIKey()
	require.NoError(t, err)
	created, err := db.CreateAPIKey(ctx, &types.ApiKey{
		Owner:          owner.ID,
		OwnerType:      types.OwnerTypeUser,
		Key:            key,
		Name:           "Asset E2E session key",
		Type:           types.APIkeytypeAPI,
		OrganizationID: orgID,
		SessionID:      session.ID,
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.DeleteAPIKey(context.Background(), created.Key)) })
	return created.Key
}

func assetCollectionPath(org string) string {
	return "/api/v1/orgs/" + url.PathEscape(org) + "/assets"
}

func assetItemPath(org, id string) string {
	return assetCollectionPath(org) + "/" + url.PathEscape(id)
}

func assetE2ERequest(t *testing.T, apiKey, method, path string, payload any, wantStatus int, output any) {
	t.Helper()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		require.NoError(t, err)
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(context.Background(), method, integrationServerURL()+path, body)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.Equal(t, wantStatus, response.StatusCode, string(data))
	if output != nil && len(data) > 0 {
		require.NoError(t, json.Unmarshal(data, output), string(data))
	}
}

type bearerRoundTripper struct {
	apiKey string
	base   http.RoundTripper
}

func (b bearerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	clone.Header = req.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+b.apiKey)
	return b.base.RoundTrip(clone)
}

func connectAssetE2EMCP(t *testing.T, apiKey string) *mcp.ClientSession {
	t.Helper()
	transport := &mcp.StreamableClientTransport{
		Endpoint:             integrationServerURL() + "/api/v1/mcp/helix-org",
		HTTPClient:           &http.Client{Transport: bearerRoundTripper{apiKey: apiKey, base: http.DefaultTransport}},
		DisableStandaloneSSE: true,
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "asset-e2e", Version: "1.0.0"}, nil)
	session, err := client.Connect(context.Background(), transport, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })
	return session
}

func callAssetE2ETool(t *testing.T, session *mcp.ClientSession, name string, arguments map[string]any, output any) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: arguments})
	require.NoError(t, err)
	require.False(t, result.IsError, "%s returned an MCP error: %+v", name, result.Content)
	require.NotEmpty(t, result.Content)
	text, ok := result.Content[0].(*mcp.TextContent)
	require.True(t, ok, "%s returned %T", name, result.Content[0])
	if output != nil {
		require.NoError(t, json.Unmarshal([]byte(text.Text), output), text.Text)
	}
}

func mcpToolNames(tools []*mcp.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, value := range tools {
		names = append(names, value.Name)
	}
	return names
}

func agentIDs(agents []orgapi.BotDTO) []string {
	ids := make([]string, 0, len(agents))
	for _, value := range agents {
		ids = append(ids, value.ID)
	}
	return ids
}

func commandIDs(commands []assetssh.Command) []string {
	ids := make([]string, 0, len(commands))
	for _, value := range commands {
		ids = append(ids, value.ID)
	}
	return ids
}

func fileNames(files []assetssh.FileEntry) []string {
	names := make([]string, 0, len(files))
	for _, value := range files {
		names = append(names, value.Name)
	}
	return names
}

func pathDir(value string) string {
	index := strings.LastIndex(value, "/")
	if index <= 0 {
		return "/"
	}
	return value[:index]
}

func assetProxyClientConfig(t *testing.T, user, privateKey, certificate string) *ssh.ClientConfig {
	t.Helper()
	privateSigner, err := ssh.ParsePrivateKey([]byte(privateKey))
	require.NoError(t, err)
	publicKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(certificate))
	require.NoError(t, err)
	cert, ok := publicKey.(*ssh.Certificate)
	require.True(t, ok)
	certSigner, err := ssh.NewCertSigner(cert, privateSigner)
	require.NoError(t, err)
	return &ssh.ClientConfig{
		User: user, Auth: []ssh.AuthMethod{ssh.PublicKeys(certSigner)}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	}
}

type assetE2ESSHServer struct {
	listener net.Listener
	config   *ssh.ServerConfig
	mu       sync.RWMutex
	key      ssh.PublicKey
	done     chan struct{}
}

func startAssetE2ESSHServer(t *testing.T, username string) *assetE2ESSHServer {
	t.Helper()
	privateKey, _, err := helixcrypto.GenerateSSHKeyPair("ed25519")
	require.NoError(t, err)
	hostSigner, err := ssh.ParsePrivateKey([]byte(privateKey))
	require.NoError(t, err)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	server := &assetE2ESSHServer{listener: listener, done: make(chan struct{})}
	server.config = &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		server.mu.RLock()
		allowed := server.key
		server.mu.RUnlock()
		if meta.User() != username || allowed == nil || !bytes.Equal(allowed.Marshal(), key.Marshal()) {
			return nil, errors.New("asset E2E SSH authentication rejected")
		}
		return nil, nil
	}}
	server.config.AddHostKey(hostSigner)
	go server.serve()
	t.Cleanup(func() {
		require.NoError(t, server.listener.Close())
		select {
		case <-server.done:
		case <-time.After(5 * time.Second):
			t.Error("asset E2E SSH server did not stop")
		}
	})
	return server
}

func (s *assetE2ESSHServer) address() string { return s.listener.Addr().String() }

func (s *assetE2ESSHServer) authorize(t *testing.T, publicKey string) {
	t.Helper()
	key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	require.NoError(t, err)
	s.mu.Lock()
	s.key = key
	s.mu.Unlock()
}

func (s *assetE2ESSHServer) serve() {
	defer close(s.done)
	for {
		connection, err := s.listener.Accept()
		if err != nil {
			return
		}
		go s.serveConnection(connection)
	}
}

func (s *assetE2ESSHServer) serveConnection(raw net.Conn) {
	defer raw.Close()
	connection, channels, requests, err := ssh.NewServerConn(raw, s.config)
	if err != nil {
		return
	}
	defer connection.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "session channels only")
			continue
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			continue
		}
		go serveAssetE2ESSHSession(channel, requests)
	}
}

func serveAssetE2ESSHSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	var commandMu sync.Mutex
	var command *exec.Cmd
	for request := range requests {
		switch request.Type {
		case "subsystem":
			var payload struct{ Name string }
			if ssh.Unmarshal(request.Payload, &payload) != nil || payload.Name != "sftp" {
				_ = request.Reply(false, nil)
				continue
			}
			_ = request.Reply(true, nil)
			server, err := sftp.NewServer(channel)
			if err == nil {
				_ = server.Serve()
				_ = server.Close()
			}
			_ = channel.Close()
			return
		case "exec":
			var payload struct{ Command string }
			if ssh.Unmarshal(request.Payload, &payload) != nil {
				_ = request.Reply(false, nil)
				continue
			}
			cmd := exec.Command("/bin/sh", "-c", payload.Command)
			cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
			cmd.Stdout = channel
			cmd.Stderr = channel.Stderr()
			if err := cmd.Start(); err != nil {
				_ = request.Reply(false, nil)
				_ = channel.Close()
				return
			}
			commandMu.Lock()
			command = cmd
			commandMu.Unlock()
			_ = request.Reply(true, nil)
			go func() {
				err := cmd.Wait()
				status := uint32(0)
				if err != nil {
					status = uint32(1)
					if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() >= 0 {
						status = uint32(cmd.ProcessState.ExitCode())
					}
				}
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: status}))
				_ = channel.Close()
			}()
		case "signal":
			var payload struct{ Signal string }
			if ssh.Unmarshal(request.Payload, &payload) != nil {
				_ = request.Reply(false, nil)
				continue
			}
			commandMu.Lock()
			cmd := command
			commandMu.Unlock()
			if cmd == nil || cmd.Process == nil {
				_ = request.Reply(false, nil)
				continue
			}
			signal := syscall.SIGTERM
			switch strings.ToUpper(payload.Signal) {
			case "KILL":
				signal = syscall.SIGKILL
			case "INT":
				signal = syscall.SIGINT
			case "HUP":
				signal = syscall.SIGHUP
			}
			err := syscall.Kill(-cmd.Process.Pid, signal)
			_ = request.Reply(err == nil, nil)
		default:
			if request.WantReply {
				_ = request.Reply(false, nil)
			}
		}
	}
}
