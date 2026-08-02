package assetssh

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/ssh"

	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
)

type Assets interface {
	Resolve(ctx context.Context, orgID, idOrName string) (asset.Asset, error)
	AuthorizeRef(ctx context.Context, orgID, agentID, idOrName string) (asset.Asset, error)
	PinHostKey(ctx context.Context, orgID string, id asset.ID, hostKey string) error
}

type Decrypt func(ciphertext string) ([]byte, error)

type CommandStatus string

const (
	CommandRunning  CommandStatus = "running"
	CommandFinished CommandStatus = "finished"
	CommandFailed   CommandStatus = "failed"
	CommandKilled   CommandStatus = "killed"
)

type Command struct {
	ID         string            `json:"id"`
	AssetID    string            `json:"asset_id"`
	Cmd        string            `json:"cmd"`
	Args       []string          `json:"args,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	Sudo       bool              `json:"sudo,omitempty"`
	Detached   bool              `json:"detached,omitempty"`
	Status     CommandStatus     `json:"status"`
	ExitCode   *int              `json:"exit_code,omitempty"`
	StartedAt  time.Time         `json:"started_at"`
	FinishedAt *time.Time        `json:"finished_at,omitempty"`
	Stdout     string            `json:"stdout,omitempty"`
	Stderr     string            `json:"stderr,omitempty"`
}

type RunRequest struct {
	Cmd            string
	Args           []string
	Cwd            string
	Env            map[string]string
	Sudo           bool
	Detached       bool
	TimeoutSeconds int
}

type Health struct {
	TCPReachable bool      `json:"tcp_reachable"`
	SSHReachable bool      `json:"ssh_reachable"`
	LatencyMS    int64     `json:"latency_ms"`
	Error        string    `json:"error,omitempty"`
	CheckedAt    time.Time `json:"checked_at"`
}

type FileEntry struct {
	Name    string    `json:"name"`
	Path    string    `json:"path"`
	IsDir   bool      `json:"is_dir"`
	Size    int64     `json:"size"`
	Mode    string    `json:"mode"`
	ModTime time.Time `json:"mod_time"`
}

type Client struct {
	assets   Assets
	decrypt  Decrypt
	newID    func() string
	now      func() time.Time
	timeout  time.Duration
	audit    orgaudit.Recorder
	projects orgaudit.ProjectResolver

	mu       sync.RWMutex
	commands map[commandKey]*trackedCommand
}

func (c *Client) WithAudit(recorder orgaudit.Recorder, projects orgaudit.ProjectResolver) *Client {
	c.audit = recorder
	c.projects = projects
	return c
}

type commandKey struct{ orgID, assetID, commandID string }

type trackedCommand struct {
	mu      sync.Mutex
	command Command
	stdout  lockedBuffer
	stderr  lockedBuffer
	session *ssh.Session
	client  *ssh.Client
	killed  bool
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

func New(assets Assets, decrypt Decrypt, newID func() string) (*Client, error) {
	if assets == nil || decrypt == nil || newID == nil {
		return nil, errors.New("asset SSH client requires assets, decrypt, and ID generator")
	}
	return &Client{
		assets: assets, decrypt: decrypt, newID: newID,
		now: func() time.Time { return time.Now().UTC() }, timeout: 10 * time.Second,
		commands: make(map[commandKey]*trackedCommand),
	}, nil
}

func (c *Client) Health(ctx context.Context, orgID, idOrName string) Health {
	checkedAt := c.now()
	h := Health{CheckedAt: checkedAt}
	a, err := c.assets.Resolve(ctx, orgID, idOrName)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	if a.Disabled {
		h.Error = fmt.Sprintf("asset %q is disabled", a.Name)
		return h
	}
	server, err := serverConfig(a)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	started := time.Now()
	conn, err := (&net.Dialer{Timeout: c.timeout}).DialContext(ctx, "tcp", serverAddress(server))
	h.LatencyMS = time.Since(started).Milliseconds()
	if err != nil {
		h.Error = err.Error()
		return h
	}
	h.TCPReachable = true
	_ = conn.Close()
	client, err := c.dial(ctx, a)
	if err != nil {
		h.Error = err.Error()
		return h
	}
	h.SSHReachable = true
	_ = client.Close()
	return h
}

func (c *Client) DialAuthorized(ctx context.Context, orgID, agentID, idOrName string) (*ssh.Client, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return nil, err
	}
	return c.dial(ctx, a)
}

func (c *Client) Run(ctx context.Context, orgID, agentID, idOrName string, req RunRequest) (Command, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return Command{}, err
	}
	remote, err := buildRemoteCommand(req)
	if err != nil {
		return Command{}, err
	}
	client, err := c.dial(ctx, a)
	if err != nil {
		c.recordConnection(ctx, orgID, agentID, a, orgaudit.StatusFailed, err)
		c.recordCommand(ctx, orgID, agentID, a.ID, "", remote, orgaudit.StatusFailed, err)
		return Command{}, err
	}
	c.recordConnection(ctx, orgID, agentID, a, orgaudit.StatusSucceeded, nil)
	session, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		c.recordCommand(ctx, orgID, agentID, a.ID, "", remote, orgaudit.StatusFailed, err)
		return Command{}, fmt.Errorf("create SSH session: %w", err)
	}
	tracked := &trackedCommand{command: Command{
		ID: c.newID(), AssetID: a.ID, Cmd: req.Cmd, Args: append([]string(nil), req.Args...),
		Cwd: req.Cwd, Env: cloneMap(req.Env), Sudo: req.Sudo, Detached: req.Detached,
		Status: CommandRunning, StartedAt: c.now(),
	}, session: session, client: client}
	session.Stdout = &tracked.stdout
	session.Stderr = &tracked.stderr
	key := commandKey{orgID: orgID, assetID: a.ID, commandID: tracked.command.ID}
	c.mu.Lock()
	c.commands[key] = tracked
	c.mu.Unlock()
	if err := session.Start(remote); err != nil {
		_ = session.Close()
		_ = client.Close()
		c.finish(tracked, err)
		c.recordCommand(ctx, orgID, agentID, a.ID, tracked.command.ID, remote, orgaudit.StatusFailed, err)
		return tracked.snapshot(), fmt.Errorf("start server command: %w", err)
	}
	c.recordCommand(ctx, orgID, agentID, a.ID, tracked.command.ID, remote, orgaudit.StatusAttempted, nil)
	if req.Detached {
		go func() {
			err := session.Wait()
			c.finish(tracked, err)
		}()
		return tracked.snapshot(), nil
	}

	timeout := time.Duration(req.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	wait := make(chan error, 1)
	go func() { wait <- session.Wait() }()
	select {
	case err := <-wait:
		c.finish(tracked, err)
		if err != nil {
			return tracked.snapshot(), fmt.Errorf("run server command: %w", err)
		}
		return tracked.snapshot(), nil
	case <-ctx.Done():
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		tracked.mu.Lock()
		tracked.killed = true
		tracked.mu.Unlock()
		c.finish(tracked, ctx.Err())
		return tracked.snapshot(), ctx.Err()
	case <-time.After(timeout):
		_ = session.Signal(ssh.SIGKILL)
		_ = session.Close()
		tracked.mu.Lock()
		tracked.killed = true
		tracked.mu.Unlock()
		c.finish(tracked, errors.New("command timed out"))
		return tracked.snapshot(), errors.New("server command timed out")
	}
}

func (c *Client) ListCommands(ctx context.Context, orgID, agentID, idOrName string) ([]Command, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return nil, err
	}
	c.mu.RLock()
	commands := make([]Command, 0)
	for key, tracked := range c.commands {
		if key.orgID == orgID && key.assetID == a.ID {
			commands = append(commands, tracked.snapshot())
		}
	}
	c.mu.RUnlock()
	sort.Slice(commands, func(i, j int) bool { return commands[i].StartedAt.After(commands[j].StartedAt) })
	return commands, nil
}

func (c *Client) GetCommand(ctx context.Context, orgID, agentID, idOrName, commandID string) (Command, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return Command{}, err
	}
	c.mu.RLock()
	tracked := c.commands[commandKey{orgID: orgID, assetID: a.ID, commandID: commandID}]
	c.mu.RUnlock()
	if tracked == nil {
		return Command{}, errors.New("server command not found")
	}
	return tracked.snapshot(), nil
}

func (c *Client) KillCommand(ctx context.Context, orgID, agentID, idOrName, commandID, signal string) error {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return err
	}
	c.mu.RLock()
	tracked := c.commands[commandKey{orgID: orgID, assetID: a.ID, commandID: commandID}]
	c.mu.RUnlock()
	if tracked == nil {
		return errors.New("server command not found")
	}
	sig := strings.ToUpper(strings.TrimSpace(signal))
	if sig == "" {
		sig = "TERM"
	}
	if sig != "TERM" && sig != "KILL" && sig != "INT" && sig != "HUP" {
		return fmt.Errorf("unsupported signal %q", signal)
	}
	tracked.mu.Lock()
	if tracked.command.Status != CommandRunning {
		tracked.mu.Unlock()
		return errors.New("server command is not running")
	}
	tracked.killed = true
	session := tracked.session
	tracked.mu.Unlock()
	if err := session.Signal(ssh.Signal(sig)); err != nil {
		return fmt.Errorf("signal server command: %w", err)
	}
	if sig == "KILL" {
		_ = session.Close()
	}
	return nil
}

func (c *Client) ListFiles(ctx context.Context, orgID, agentID, idOrName, directory string) ([]FileEntry, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return nil, err
	}
	directory, err = cleanAbsolutePath(directory)
	if err != nil {
		return nil, err
	}
	var entries []FileEntry
	err = c.withSFTP(ctx, orgID, agentID, a, func(client *sftp.Client) error {
		infos, err := client.ReadDir(directory)
		if err != nil {
			return err
		}
		entries = make([]FileEntry, 0, len(infos))
		for _, info := range infos {
			entries = append(entries, FileEntry{
				Name: info.Name(), Path: path.Join(directory, info.Name()), IsDir: info.IsDir(),
				Size: info.Size(), Mode: info.Mode().String(), ModTime: info.ModTime(),
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list server files: %w", err)
	}
	return entries, nil
}

func (c *Client) ReadFile(ctx context.Context, orgID, agentID, idOrName, filename string, maxBytes int64) ([]byte, error) {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return nil, err
	}
	filename, err = cleanAbsolutePath(filename)
	if err != nil {
		return nil, err
	}
	if maxBytes <= 0 {
		maxBytes = 1024 * 1024
	}
	var data []byte
	err = c.withSFTP(ctx, orgID, agentID, a, func(client *sftp.Client) error {
		file, err := client.Open(filename)
		if err != nil {
			return err
		}
		defer file.Close()
		data, err = io.ReadAll(io.LimitReader(file, maxBytes+1))
		if err != nil {
			return err
		}
		if int64(len(data)) > maxBytes {
			return fmt.Errorf("file exceeds %d byte read limit", maxBytes)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read server file: %w", err)
	}
	return data, nil
}

func (c *Client) WriteFile(ctx context.Context, orgID, agentID, idOrName, filename string, data []byte, mode uint32) error {
	a, err := c.assets.AuthorizeRef(ctx, orgID, agentID, idOrName)
	if err != nil {
		return err
	}
	filename, err = cleanAbsolutePath(filename)
	if err != nil {
		return err
	}
	return c.withSFTP(ctx, orgID, agentID, a, func(client *sftp.Client) error {
		if err := client.MkdirAll(path.Dir(filename)); err != nil {
			return fmt.Errorf("create parent directories: %w", err)
		}
		file, err := client.Create(filename)
		if err != nil {
			return err
		}
		if _, err := file.Write(data); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if mode != 0 {
			if err := client.Chmod(filename, os.FileMode(mode)); err != nil {
				return err
			}
		}
		return nil
	})
}

func (c *Client) withSFTP(ctx context.Context, orgID, agentID string, a asset.Asset, fn func(*sftp.Client) error) error {
	client, err := c.dial(ctx, a)
	if err != nil {
		c.recordConnection(ctx, orgID, agentID, a, orgaudit.StatusFailed, err)
		return err
	}
	c.recordConnection(ctx, orgID, agentID, a, orgaudit.StatusSucceeded, nil)
	defer client.Close()
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		return fmt.Errorf("start SFTP client: %w", err)
	}
	defer sftpClient.Close()
	return fn(sftpClient)
}

func (c *Client) recordConnection(ctx context.Context, orgID, agentID string, a asset.Asset, status orgaudit.Status, connectionErr error) {
	server, _ := serverConfig(a)
	metadata := orgaudit.Metadata{RemoteAddress: serverAddress(server), SSHUser: server.User}
	if connectionErr != nil {
		metadata.Error = connectionErr.Error()
	}
	c.recordAudit(ctx, orgaudit.Entry{
		OrganizationID: orgID,
		ProjectID:      c.projectID(ctx, orgID, agentID),
		ActorID:        agentID,
		ActorType:      orgaudit.ActorBot,
		AssetID:        string(a.ID),
		EventType:      orgaudit.EventSSHConnection,
		Action:         "connect",
		Status:         status,
		Metadata:       metadata,
	})
}

func (c *Client) recordCommand(ctx context.Context, orgID, agentID string, assetID asset.ID, commandID, command string, status orgaudit.Status, commandErr error) {
	metadata := orgaudit.Metadata{Command: command, CommandID: commandID}
	if commandErr != nil {
		metadata.Error = commandErr.Error()
	}
	c.recordAudit(ctx, orgaudit.Entry{
		OrganizationID: orgID,
		ProjectID:      c.projectID(ctx, orgID, agentID),
		ActorID:        agentID,
		ActorType:      orgaudit.ActorBot,
		AssetID:        string(assetID),
		EventType:      orgaudit.EventSSHCommand,
		Action:         "exec",
		Status:         status,
		Metadata:       metadata,
	})
}

func (c *Client) projectID(ctx context.Context, orgID, agentID string) string {
	if c.projects == nil {
		return ""
	}
	projectID, err := c.projects(ctx, orgID, agentID)
	if err != nil {
		log.Error().Err(err).Str("organization_id", orgID).Str("actor_id", agentID).Msg("failed to resolve org audit project")
		return ""
	}
	return projectID
}

func (c *Client) recordAudit(ctx context.Context, entry orgaudit.Entry) {
	if c.audit == nil {
		return
	}
	if err := c.audit.Record(ctx, entry); err != nil {
		log.Error().Err(err).Str("organization_id", entry.OrganizationID).Str("event_type", string(entry.EventType)).Msg("failed to create org audit log")
	}
}

func (c *Client) dial(ctx context.Context, a asset.Asset) (*ssh.Client, error) {
	server, err := serverConfig(a)
	if err != nil {
		return nil, err
	}
	var authMethod ssh.AuthMethod
	switch server.AuthType {
	case asset.AuthSSHKey:
		privateKey, err := c.decrypt(server.EncryptedPrivateKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt server SSH private key: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(privateKey)
		if err != nil {
			return nil, fmt.Errorf("parse server SSH private key: %w", err)
		}
		authMethod = ssh.PublicKeys(signer)
	case asset.AuthPassword:
		password, err := c.decrypt(server.EncryptedPassword)
		if err != nil {
			return nil, fmt.Errorf("decrypt server SSH password: %w", err)
		}
		authMethod = ssh.Password(string(password))
	default:
		return nil, fmt.Errorf("unsupported server auth type %q", server.AuthType)
	}
	var captured ssh.PublicKey
	hostKeyCallback := func(_ string, _ net.Addr, key ssh.PublicKey) error {
		if server.HostKey == "" {
			captured = key
			return nil
		}
		pinned, _, _, _, err := ssh.ParseAuthorizedKey([]byte(server.HostKey))
		if err != nil {
			return fmt.Errorf("parse pinned server host key: %w", err)
		}
		if !bytes.Equal(pinned.Marshal(), key.Marshal()) {
			return errors.New("server SSH host key changed")
		}
		return nil
	}
	config := &ssh.ClientConfig{
		User: server.User, Auth: []ssh.AuthMethod{authMethod},
		HostKeyCallback: hostKeyCallback, Timeout: c.timeout,
	}
	netConn, err := (&net.Dialer{Timeout: c.timeout}).DialContext(ctx, "tcp", serverAddress(server))
	if err != nil {
		return nil, fmt.Errorf("connect to server TCP endpoint: %w", err)
	}
	deadline := time.Now().Add(c.timeout)
	_ = netConn.SetDeadline(deadline)
	conn, chans, reqs, err := ssh.NewClientConn(netConn, serverAddress(server), config)
	if err != nil {
		_ = netConn.Close()
		return nil, fmt.Errorf("connect to server SSH endpoint: %w", err)
	}
	_ = netConn.SetDeadline(time.Time{})
	client := ssh.NewClient(conn, chans, reqs)
	if captured != nil {
		hostKey := strings.TrimSpace(string(ssh.MarshalAuthorizedKey(captured)))
		if err := c.assets.PinHostKey(ctx, a.OrganizationID, a.ID, hostKey); err != nil {
			_ = client.Close()
			return nil, fmt.Errorf("pin server SSH host key: %w", err)
		}
	}
	return client, nil
}

func (c *Client) finish(tracked *trackedCommand, runErr error) {
	now := c.now()
	tracked.mu.Lock()
	defer tracked.mu.Unlock()
	if tracked.command.FinishedAt != nil {
		return
	}
	tracked.command.FinishedAt = &now
	tracked.command.Stdout = tracked.stdout.String()
	tracked.command.Stderr = tracked.stderr.String()
	if tracked.killed {
		tracked.command.Status = CommandKilled
	} else if runErr == nil {
		tracked.command.Status = CommandFinished
		code := 0
		tracked.command.ExitCode = &code
	} else {
		tracked.command.Status = CommandFailed
		var exitErr *ssh.ExitError
		if errors.As(runErr, &exitErr) {
			code := exitErr.ExitStatus()
			tracked.command.ExitCode = &code
		}
	}
	_ = tracked.session.Close()
	_ = tracked.client.Close()
}

func (t *trackedCommand) snapshot() Command {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.command
	if out.FinishedAt == nil {
		out.Stdout = t.stdout.String()
		out.Stderr = t.stderr.String()
	}
	out.Args = append([]string(nil), out.Args...)
	out.Env = cloneMap(out.Env)
	return out
}

var envNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func buildRemoteCommand(req RunRequest) (string, error) {
	if strings.TrimSpace(req.Cmd) == "" || strings.ContainsRune(req.Cmd, 0) {
		return "", errors.New("server command is empty or invalid")
	}
	parts := make([]string, 0, len(req.Args)+len(req.Env)+5)
	if req.Sudo {
		parts = append(parts, "sudo", "--")
	}
	if len(req.Env) > 0 {
		keys := make([]string, 0, len(req.Env))
		for key := range req.Env {
			if !envNamePattern.MatchString(key) {
				return "", fmt.Errorf("invalid environment variable name %q", key)
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		parts = append(parts, "env")
		for _, key := range keys {
			parts = append(parts, key+"="+shellQuote(req.Env[key]))
		}
	}
	parts = append(parts, shellQuote(req.Cmd))
	for _, arg := range req.Args {
		if strings.ContainsRune(arg, 0) {
			return "", errors.New("server command argument contains NUL")
		}
		parts = append(parts, shellQuote(arg))
	}
	command := strings.Join(parts, " ")
	if req.Cwd != "" {
		cwd, err := cleanAbsolutePath(req.Cwd)
		if err != nil {
			return "", fmt.Errorf("invalid server command cwd: %w", err)
		}
		command = "cd -- " + shellQuote(cwd) + " && " + command
	}
	return command, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func cleanAbsolutePath(value string) (string, error) {
	if strings.ContainsRune(value, 0) || !strings.HasPrefix(value, "/") {
		return "", errors.New("path must be absolute and contain no NUL")
	}
	return path.Clean(value), nil
}

func serverConfig(a asset.Asset) (asset.Server, error) {
	if a.Kind != asset.KindServer || a.Config.Server == nil {
		return asset.Server{}, fmt.Errorf("asset %q is not a server", a.ID)
	}
	return *a.Config.Server, nil
}

func serverAddress(server asset.Server) string {
	return net.JoinHostPort(server.Address, fmt.Sprintf("%d", server.Port))
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
