package assetssh

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/org/domain/asset"
	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

type recordingAudit struct {
	entries      chan orgaudit.Entry
	beforeRecord func(orgaudit.Entry)
}

func newRecordingAudit() *recordingAudit {
	return &recordingAudit{entries: make(chan orgaudit.Entry, 16)}
}

func (r *recordingAudit) Record(_ context.Context, entry orgaudit.Entry) error {
	if r.beforeRecord != nil {
		r.beforeRecord(entry)
	}
	r.entries <- entry
	return nil
}

type proxyAssets struct {
	value   asset.Asset
	mu      sync.RWMutex
	allowed bool
}

func (p *proxyAssets) Resolve(context.Context, string, string) (asset.Asset, error) {
	return p.value, nil
}
func (p *proxyAssets) AuthorizeRef(_ context.Context, orgID, agentID, ref string) (asset.Asset, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if !p.allowed || orgID != p.value.OrganizationID || agentID != "agent-1" || (ref != p.value.ID && ref != p.value.Name) {
		return asset.Asset{}, errors.New("asset is not linked to agent")
	}
	if p.value.Disabled {
		return asset.Asset{}, errors.New("asset is disabled")
	}
	return p.value, nil
}
func (*proxyAssets) PinHostKey(context.Context, string, asset.ID, string) error { return nil }
func (p *proxyAssets) setAllowed(value bool) {
	p.mu.Lock()
	p.allowed = value
	p.mu.Unlock()
}
func (p *proxyAssets) setDisabled(value bool) {
	p.mu.Lock()
	p.value.Disabled = value
	p.mu.Unlock()
}

type connMetadata struct{ user string }

func (m connMetadata) User() string          { return m.user }
func (m connMetadata) SessionID() []byte     { return nil }
func (m connMetadata) ClientVersion() []byte { return []byte("SSH-2.0-test") }
func (m connMetadata) ServerVersion() []byte { return []byte("SSH-2.0-test") }
func (m connMetadata) RemoteAddr() net.Addr  { return &net.TCPAddr{} }
func (m connMetadata) LocalAddr() net.Addr   { return &net.TCPAddr{} }

func TestProxyCertificateEnforcesAgentLinkAndAssetUsername(t *testing.T) {
	assets := &proxyAssets{allowed: true, value: asset.Asset{
		ID: "a-prod", OrganizationID: "org-1", Name: "production", Kind: asset.KindServer,
		Config: asset.Config{Server: &asset.Server{Address: "10.0.0.1", Port: 22, User: "ubuntu", AuthType: asset.AuthSSHKey, PublicKey: "key", EncryptedPrivateKey: "secret"}},
	}}
	key := []byte("01234567890123456789012345678901")
	issuer, err := NewIssuer(assets, key)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := issuer.Mint(context.Background(), "org-1", "agent-1", "production")
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := ssh.ParsePrivateKey([]byte(identity.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	certKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(identity.Certificate))
	if err != nil {
		t.Fatal(err)
	}
	cert, ok := certKey.(*ssh.Certificate)
	if !ok {
		t.Fatal("minted public key is not an SSH certificate")
	}
	if string(cert.Key.Marshal()) != string(privateKey.PublicKey().Marshal()) {
		t.Fatal("certificate does not match private key")
	}

	client, err := New(assets, func(string) ([]byte, error) { return nil, nil }, func() string { return "cmd" })
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(assets, client, key)
	if err != nil {
		t.Fatal(err)
	}
	auditLog := newRecordingAudit()
	proxy.WithAudit(auditLog, func(context.Context, string, string) (string, error) { return "project-1", nil })
	if _, err := proxy.config.PublicKeyCallback(connMetadata{user: "production"}, cert); err != nil {
		t.Fatalf("valid certificate rejected: %v", err)
	}
	if _, err := proxy.config.PublicKeyCallback(connMetadata{user: "staging"}, cert); err == nil {
		t.Fatal("certificate accepted for a different asset username")
	}
	assets.setDisabled(true)
	if _, err := proxy.config.PublicKeyCallback(connMetadata{user: "production"}, cert); err == nil {
		t.Fatal("certificate remained usable after the asset was disabled")
	}
	assets.setDisabled(false)
	if _, err := proxy.config.PublicKeyCallback(connMetadata{user: "production"}, cert); err != nil {
		t.Fatalf("certificate did not become usable after the asset was re-enabled: %v", err)
	}
	assets.setAllowed(false)
	if _, err := proxy.config.PublicKeyCallback(connMetadata{user: "production"}, cert); err == nil {
		t.Fatal("certificate remained usable after the agent link was revoked")
	}
	select {
	case entry := <-auditLog.entries:
		if entry.EventType != orgaudit.EventSSHConnection || entry.Status != orgaudit.StatusFailed || entry.OrganizationID != "org-1" || entry.ProjectID != "project-1" || entry.AssetID != "a-prod" || entry.Metadata.SSHUser != "production" {
			t.Fatalf("unexpected rejected connection audit: %+v", entry)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("rejected SSH connection was not audited")
	}
}

func TestParseProxyAddress(t *testing.T) {
	host, port, err := ParseProxyAddress("helix.example.com:2224")
	if err != nil || host != "helix.example.com" || port != 2224 {
		t.Fatalf("got host=%q port=%d err=%v", host, port, err)
	}
	if _, _, err := ParseProxyAddress("missing-port"); err == nil {
		t.Fatal("expected missing port to fail")
	}
}

func TestProxyForwardsAuthorizedAgentSessionAndRejectsRevokedLink(t *testing.T) {
	privateKey, publicKey, err := helixcrypto.GenerateSSHKeyPair("ed25519")
	if err != nil {
		t.Fatal(err)
	}
	authorizedKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(publicKey))
	if err != nil {
		t.Fatal(err)
	}
	upstreamAddress, commands, stopUpstream := startProxyTestUpstream(t, authorizedKey)
	defer stopUpstream()
	host, rawPort, err := net.SplitHostPort(upstreamAddress)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatal(err)
	}

	assets := &proxyAssets{allowed: true, value: asset.Asset{
		ID: "a-prod", OrganizationID: "org-1", Name: "production", Kind: asset.KindServer,
		Config: asset.Config{Server: &asset.Server{
			Address: host, Port: uint16(port), User: "ubuntu", AuthType: asset.AuthSSHKey,
			PublicKey: publicKey, EncryptedPrivateKey: privateKey,
		}},
	}}
	key := []byte("01234567890123456789012345678901")
	issuer, err := NewIssuer(assets, key)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := issuer.Mint(context.Background(), "org-1", "agent-1", "production")
	if err != nil {
		t.Fatal(err)
	}
	privateSigner, err := ssh.ParsePrivateKey([]byte(identity.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	certificateKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(identity.Certificate))
	if err != nil {
		t.Fatal(err)
	}
	certificate, ok := certificateKey.(*ssh.Certificate)
	if !ok {
		t.Fatal("minted identity is not an SSH certificate")
	}
	certificateSigner, err := ssh.NewCertSigner(certificate, privateSigner)
	if err != nil {
		t.Fatal(err)
	}

	client, err := New(assets, func(ciphertext string) ([]byte, error) { return []byte(ciphertext), nil }, func() string { return "command-1" })
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(assets, client, key)
	if err != nil {
		t.Fatal(err)
	}
	auditLog := newRecordingAudit()
	auditStarted := make(chan struct{})
	auditRelease := make(chan struct{})
	var releaseAudit sync.Once
	release := func() { releaseAudit.Do(func() { close(auditRelease) }) }
	t.Cleanup(release)
	auditLog.beforeRecord = func(entry orgaudit.Entry) {
		if entry.EventType == orgaudit.EventSSHCommand {
			close(auditStarted)
			<-auditRelease
		}
	}
	proxy.WithAudit(auditLog, func(_ context.Context, orgID, agentID string) (string, error) {
		if orgID != "org-1" || agentID != "agent-1" {
			return "", errors.New("unexpected audit project scope")
		}
		return "project-1", nil
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- proxy.serve(ctx, listener) }()

	config := &ssh.ClientConfig{
		User: "production", Auth: []ssh.AuthMethod{ssh.PublicKeys(certificateSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	}
	proxyClient, err := ssh.Dial("tcp", listener.Addr().String(), config)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	session, err := proxyClient.NewSession()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	type commandResult struct {
		output []byte
		err    error
	}
	resultCh := make(chan commandResult, 1)
	go func() {
		output, err := session.CombinedOutput("printf proxy-ok")
		resultCh <- commandResult{output: output, err: err}
	}()
	select {
	case <-auditStarted:
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("SSH command audit did not start")
	}
	var output []byte
	select {
	case result := <-resultCh:
		if result.err != nil {
			cancel()
			t.Fatalf("SSH command failed while audit persistence was blocked: %v", result.err)
		}
		output = result.output
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("SSH command reply was blocked by audit persistence")
	}
	release()
	_ = proxyClient.Close()
	if string(output) != "proxied:printf proxy-ok" {
		t.Fatalf("proxy output = %q", output)
	}
	select {
	case command := <-commands:
		if command != "printf proxy-ok" {
			t.Fatalf("upstream command = %q", command)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("upstream did not receive proxied command")
	}

	audited := make(map[orgaudit.EventType]orgaudit.Entry)
	for len(audited) < 2 {
		select {
		case entry := <-auditLog.entries:
			audited[entry.EventType] = entry
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for SSH audit entries: %+v", audited)
		}
	}
	connection := audited[orgaudit.EventSSHConnection]
	if connection.OrganizationID != "org-1" || connection.ProjectID != "project-1" || connection.ActorID != "agent-1" || connection.AssetID != "a-prod" || connection.Status != orgaudit.StatusSucceeded {
		t.Fatalf("unexpected connection audit: %+v", connection)
	}
	if connection.Metadata.RemoteAddress == "" || connection.Metadata.SSHUser != "production" {
		t.Fatalf("connection metadata missing: %+v", connection.Metadata)
	}
	commandAudit := audited[orgaudit.EventSSHCommand]
	if commandAudit.Metadata.Command != "printf proxy-ok" || commandAudit.Status != orgaudit.StatusAttempted {
		t.Fatalf("unexpected command audit: %+v", commandAudit)
	}

	assets.setAllowed(false)
	if revokedClient, err := ssh.Dial("tcp", listener.Addr().String(), config); err == nil {
		_ = revokedClient.Close()
		t.Fatal("revoked agent link still opened the SSH proxy")
	}
	cancel()
	select {
	case err := <-proxyDone:
		if err != nil {
			t.Fatalf("proxy shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not stop after context cancellation")
	}
}

type sandboxTerminalFrame struct {
	messageType int
	data        []byte
}

type proxySandboxTerminal struct {
	reads  chan sandboxTerminalFrame
	writes chan sandboxTerminalFrame
}

func (t *proxySandboxTerminal) ReadMessage() (int, []byte, error) {
	frame, ok := <-t.reads
	if !ok {
		return 0, nil, io.EOF
	}
	return frame.messageType, frame.data, nil
}
func (t *proxySandboxTerminal) WriteMessage(messageType int, data []byte) error {
	t.writes <- sandboxTerminalFrame{messageType: messageType, data: append([]byte(nil), data...)}
	return nil
}
func (*proxySandboxTerminal) Close() error { return nil }

type proxySandboxes struct {
	mu      sync.RWMutex
	allowed bool
	opened  chan string
	writes  chan sandboxTerminalFrame
}

func (s *proxySandboxes) AuthorizeSSH(_ context.Context, orgID, agentID, sandboxID string) (runtime.SandboxView, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.allowed || orgID != "org-1" || agentID != "agent-1" || sandboxID != "sbx-1" {
		return runtime.SandboxView{}, errors.New("sandbox access denied")
	}
	return runtime.SandboxView{ID: sandboxID, OrganizationID: orgID, Status: "running"}, nil
}
func (s *proxySandboxes) OpenSSHTerminal(_ context.Context, orgID, agentID, sandboxID, shell string) (runtime.SandboxTerminal, error) {
	if _, err := s.AuthorizeSSH(context.Background(), orgID, agentID, sandboxID); err != nil {
		return nil, err
	}
	s.opened <- shell
	reads := make(chan sandboxTerminalFrame, 2)
	reads <- sandboxTerminalFrame{messageType: websocket.BinaryMessage, data: []byte("sandbox-ok")}
	reads <- sandboxTerminalFrame{messageType: websocket.TextMessage, data: []byte(`{"type":"exit","code":0}`)}
	return &proxySandboxTerminal{reads: reads, writes: s.writes}, nil
}
func (s *proxySandboxes) setAllowed(value bool) {
	s.mu.Lock()
	s.allowed = value
	s.mu.Unlock()
}

func TestProxyBridgesNativeSSHToAuthorizedSandboxTerminal(t *testing.T) {
	assets := &proxyAssets{allowed: true, value: asset.Asset{
		ID: "unused", OrganizationID: "org-1", Name: "unused", Kind: asset.KindServer,
	}}
	client, err := New(assets, func(string) ([]byte, error) { return nil, nil }, func() string { return "cmd" })
	if err != nil {
		t.Fatal(err)
	}
	sandboxes := &proxySandboxes{
		allowed: true,
		opened:  make(chan string, 1),
		writes:  make(chan sandboxTerminalFrame, 4),
	}
	key := []byte("01234567890123456789012345678901")
	issuer, err := NewSandboxIssuer(sandboxes, key)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := issuer.Mint(context.Background(), "org-1", "agent-1", "sbx-1")
	if err != nil {
		t.Fatal(err)
	}
	privateSigner, err := ssh.ParsePrivateKey([]byte(identity.PrivateKey))
	if err != nil {
		t.Fatal(err)
	}
	certificateKey, _, _, _, err := ssh.ParseAuthorizedKey([]byte(identity.Certificate))
	if err != nil {
		t.Fatal(err)
	}
	certificate, ok := certificateKey.(*ssh.Certificate)
	if !ok {
		t.Fatal("minted sandbox identity is not an SSH certificate")
	}
	certificateSigner, err := ssh.NewCertSigner(certificate, privateSigner)
	if err != nil {
		t.Fatal(err)
	}
	proxy, err := NewProxy(assets, client, key)
	if err != nil {
		t.Fatal(err)
	}
	proxy.WithSandboxes(sandboxes)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	proxyDone := make(chan error, 1)
	go func() { proxyDone <- proxy.serve(ctx, listener) }()

	config := &ssh.ClientConfig{
		User: "sandbox", Auth: []ssh.AuthMethod{ssh.PublicKeys(certificateSigner)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second,
	}
	proxyClient, err := ssh.Dial("tcp", listener.Addr().String(), config)
	if err != nil {
		t.Fatal(err)
	}
	session, err := proxyClient.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := session.RequestPty("xterm-256color", 24, 80, ssh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	output, err := session.CombinedOutput("printf 'sandbox-ok'")
	if err != nil {
		t.Fatalf("sandbox SSH command failed: %v", err)
	}
	if string(output) != "sandbox-ok" {
		t.Fatalf("sandbox SSH output = %q", output)
	}
	select {
	case shell := <-sandboxes.opened:
		if shell != "exec /bin/sh -c "+shellQuote("printf 'sandbox-ok'") {
			t.Fatalf("sandbox terminal shell = %q", shell)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sandbox terminal was not opened")
	}
	select {
	case frame := <-sandboxes.writes:
		var resize struct {
			Type string `json:"type"`
			Cols uint32 `json:"cols"`
			Rows uint32 `json:"rows"`
		}
		if frame.messageType != websocket.TextMessage || json.Unmarshal(frame.data, &resize) != nil || resize.Type != "resize" || resize.Cols != 80 || resize.Rows != 24 {
			t.Fatalf("sandbox resize frame = type %d data %s", frame.messageType, frame.data)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("sandbox terminal did not receive PTY dimensions")
	}
	_ = proxyClient.Close()

	sandboxes.setAllowed(false)
	if revokedClient, err := ssh.Dial("tcp", listener.Addr().String(), config); err == nil {
		_ = revokedClient.Close()
		t.Fatal("revoked sandbox certificate still opened the SSH proxy")
	}
	cancel()
	select {
	case err := <-proxyDone:
		if err != nil {
			t.Fatalf("proxy shutdown: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("proxy did not stop after context cancellation")
	}
}

func startProxyTestUpstream(t *testing.T, authorizedKey ssh.PublicKey) (string, <-chan string, func()) {
	t.Helper()
	privateKey, _, err := helixcrypto.GenerateSSHKeyPair("ed25519")
	if err != nil {
		t.Fatal(err)
	}
	hostSigner, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		if meta.User() != "ubuntu" || !bytes.Equal(key.Marshal(), authorizedKey.Marshal()) {
			return nil, errors.New("upstream authentication rejected")
		}
		return nil, nil
	}}
	config.AddHostKey(hostSigner)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	commands := make(chan string, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go serveProxyTestUpstreamConnection(raw, config, commands)
		}
	}()
	stop := func() {
		_ = listener.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("upstream SSH test server did not stop")
		}
	}
	return listener.Addr().String(), commands, stop
}

func serveProxyTestUpstreamConnection(raw net.Conn, config *ssh.ServerConfig, commands chan<- string) {
	defer raw.Close()
	conn, channels, requests, err := ssh.NewServerConn(raw, config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)
	for incoming := range channels {
		if incoming.ChannelType() != "session" {
			_ = incoming.Reject(ssh.UnknownChannelType, "session required")
			continue
		}
		channel, requests, err := incoming.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for request := range requests {
				if request.Type != "exec" {
					if request.WantReply {
						_ = request.Reply(false, nil)
					}
					continue
				}
				var payload struct{ Command string }
				if err := ssh.Unmarshal(request.Payload, &payload); err != nil {
					_ = request.Reply(false, nil)
					continue
				}
				commands <- payload.Command
				_ = request.Reply(true, nil)
				_, _ = channel.Write([]byte("proxied:" + payload.Command))
				_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: 0}))
				return
			}
		}()
	}
}
