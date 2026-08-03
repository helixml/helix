package assetssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"golang.org/x/crypto/ssh"

	orgaudit "github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

const sandboxSSHUser = "sandbox"

type SandboxAccess interface {
	AuthorizeSSH(ctx context.Context, orgID, agentID, sandboxID string) (runtime.SandboxView, error)
	OpenSSHTerminal(ctx context.Context, orgID, agentID, sandboxID, shell string) (runtime.SandboxTerminal, error)
}

type SandboxProxyIdentity struct {
	SandboxID   string    `json:"sandbox_id"`
	PrivateKey  string    `json:"private_key"`
	Certificate string    `json:"certificate"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type SandboxIssuer struct {
	sandboxes SandboxAccess
	ca        ssh.Signer
	now       func() time.Time
}

func NewSandboxIssuer(sandboxes SandboxAccess, encryptionKey []byte) (*SandboxIssuer, error) {
	if sandboxes == nil || len(encryptionKey) == 0 {
		return nil, errors.New("sandbox SSH issuer requires sandbox access and an encryption key")
	}
	ca, err := deterministicSigner(encryptionKey, "helix-asset-ssh-ca")
	if err != nil {
		return nil, err
	}
	return &SandboxIssuer{sandboxes: sandboxes, ca: ca, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (i *SandboxIssuer) Mint(ctx context.Context, orgID, agentID, sandboxID string) (SandboxProxyIdentity, error) {
	value, err := i.sandboxes.AuthorizeSSH(ctx, orgID, agentID, sandboxID)
	if err != nil {
		return SandboxProxyIdentity{}, err
	}
	if value.Status != "running" {
		return SandboxProxyIdentity{}, fmt.Errorf("sandbox %s is not running (status=%s)", value.ID, value.Status)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SandboxProxyIdentity{}, fmt.Errorf("generate sandbox proxy key: %w", err)
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return SandboxProxyIdentity{}, fmt.Errorf("encode sandbox proxy public key: %w", err)
	}
	now := i.now()
	expiresAt := now.Add(time.Hour)
	cert := &ssh.Certificate{
		Key: sshPublicKey, CertType: ssh.UserCert, KeyId: agentID,
		ValidPrincipals: []string{sandboxSSHUser},
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()),
		ValidBefore:     uint64(expiresAt.Unix()),
		Permissions: ssh.Permissions{Extensions: map[string]string{
			certOrgID: orgID, certAgentID: agentID, certTargetKind: targetKindSandbox, certSandboxID: value.ID,
		}},
	}
	if err := cert.SignCert(rand.Reader, i.ca); err != nil {
		return SandboxProxyIdentity{}, fmt.Errorf("sign sandbox proxy certificate: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return SandboxProxyIdentity{}, fmt.Errorf("marshal sandbox proxy private key: %w", err)
	}
	return SandboxProxyIdentity{
		SandboxID:   value.ID,
		PrivateKey:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
		Certificate: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert))),
		ExpiresAt:   expiresAt,
	}, nil
}

func (p *Proxy) proxySandboxSession(ctx context.Context, conn *ssh.ServerConn, incoming ssh.NewChannel) {
	channel, requests, err := incoming.Accept()
	if err != nil {
		return
	}
	defer channel.Close()

	var cols, rows uint32
	for request := range requests {
		switch request.Type {
		case "pty-req":
			var req struct {
				Term                      string
				Columns, Rows             uint32
				WidthPixels, HeightPixels uint32
				Modes                     string
			}
			if err := ssh.Unmarshal(request.Payload, &req); err != nil {
				replySSHRequest(request, false)
				continue
			}
			cols, rows = req.Columns, req.Rows
			replySSHRequest(request, true)
		case "shell", "exec":
			command := ""
			shell := ""
			if request.Type == "exec" {
				command, err = decodeExecRequest(request.Payload)
				if err != nil {
					replySSHRequest(request, false)
					return
				}
				shell = "exec /bin/sh -c " + shellQuote(command)
			}
			permissions := conn.Permissions.Extensions
			terminal, openErr := p.sandboxes.OpenSSHTerminal(ctx, permissions[certOrgID], permissions[certAgentID], permissions[certSandboxID], shell)
			if openErr != nil {
				replySSHRequest(request, false)
				_, _ = channel.Stderr().Write([]byte(openErr.Error() + "\n"))
				return
			}
			replySSHRequest(request, true)
			if command != "" {
				p.recordSandboxCommand(ctx, permissions, command)
			}
			p.bridgeSandboxTerminal(channel, requests, terminal, cols, rows)
			return
		default:
			replySSHRequest(request, false)
		}
	}
}

func (p *Proxy) bridgeSandboxTerminal(channel ssh.Channel, requests <-chan *ssh.Request, terminal runtime.SandboxTerminal, cols, rows uint32) {
	defer terminal.Close()
	writes := &lockedTerminalWriter{terminal: terminal}
	if cols > 0 && rows > 0 {
		_ = writes.resize(cols, rows)
	}
	go func() {
		buffer := make([]byte, 32*1024)
		for {
			n, err := channel.Read(buffer)
			if n > 0 {
				if writeErr := writes.writeBinary(buffer[:n]); writeErr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		for request := range requests {
			if request.Type != "window-change" {
				replySSHRequest(request, false)
				continue
			}
			var req struct{ Columns, Rows, WidthPixels, HeightPixels uint32 }
			if err := ssh.Unmarshal(request.Payload, &req); err != nil {
				replySSHRequest(request, false)
				continue
			}
			err := writes.resize(req.Columns, req.Rows)
			replySSHRequest(request, err == nil)
		}
	}()

	exitCode := uint32(255)
	for {
		messageType, data, err := terminal.ReadMessage()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_, _ = channel.Stderr().Write([]byte(err.Error() + "\n"))
			}
			break
		}
		if messageType == websocket.BinaryMessage {
			if _, err := channel.Write(data); err != nil {
				return
			}
			continue
		}
		if messageType != websocket.TextMessage {
			continue
		}
		var control struct {
			Type    string `json:"type"`
			Message string `json:"message"`
			Code    int    `json:"code"`
		}
		if err := json.Unmarshal(data, &control); err != nil {
			continue
		}
		switch control.Type {
		case "error":
			exitCode = 1
			_, _ = channel.Stderr().Write([]byte(control.Message + "\n"))
		case "exit":
			if control.Code >= 0 {
				exitCode = uint32(control.Code)
			}
			_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: exitCode}))
			return
		}
	}
	_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{Status: exitCode}))
}

type lockedTerminalWriter struct {
	mu       sync.Mutex
	terminal runtime.SandboxTerminal
}

func (w *lockedTerminalWriter) writeBinary(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminal.WriteMessage(websocket.BinaryMessage, data)
}

func (w *lockedTerminalWriter) resize(cols, rows uint32) error {
	data, err := json.Marshal(map[string]any{"type": "resize", "cols": cols, "rows": rows})
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.terminal.WriteMessage(websocket.TextMessage, data)
}

func replySSHRequest(request *ssh.Request, ok bool) {
	if request.WantReply {
		_ = request.Reply(ok, nil)
	}
}

func (p *Proxy) recordSandboxCommand(ctx context.Context, permissions map[string]string, command string) {
	p.recordAudit(ctx, orgaudit.Entry{
		OrganizationID: permissions[certOrgID],
		ProjectID:      p.projectID(ctx, permissions[certOrgID], permissions[certAgentID]),
		ActorID:        permissions[certAgentID],
		ActorType:      orgaudit.ActorBot,
		EventType:      orgaudit.EventSSHCommand,
		Action:         "exec",
		Status:         orgaudit.StatusAttempted,
		Metadata: orgaudit.Metadata{
			AssetRef: "sandbox:" + permissions[certSandboxID],
			Command:  command,
		},
	})
}
