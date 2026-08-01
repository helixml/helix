package assetssh

import (
	"context"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	certOrgID   = "helix.org_id"
	certAgentID = "helix.agent_id"
	certAssetID = "helix.asset_id"
)

type ProxyIdentity struct {
	AssetID     string    `json:"asset_id"`
	AssetName   string    `json:"asset_name"`
	PrivateKey  string    `json:"private_key"`
	Certificate string    `json:"certificate"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type Issuer struct {
	assets Assets
	ca     ssh.Signer
	now    func() time.Time
}

func NewIssuer(assets Assets, encryptionKey []byte) (*Issuer, error) {
	if assets == nil || len(encryptionKey) == 0 {
		return nil, errors.New("asset SSH issuer requires assets and an encryption key")
	}
	ca, err := deterministicSigner(encryptionKey, "helix-asset-ssh-ca")
	if err != nil {
		return nil, err
	}
	return &Issuer{assets: assets, ca: ca, now: func() time.Time { return time.Now().UTC() }}, nil
}

func (i *Issuer) Mint(ctx context.Context, orgID, agentID, assetRef string) (ProxyIdentity, error) {
	a, err := i.assets.AuthorizeRef(ctx, orgID, agentID, assetRef)
	if err != nil {
		return ProxyIdentity{}, err
	}
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return ProxyIdentity{}, fmt.Errorf("generate asset proxy key: %w", err)
	}
	sshPublicKey, err := ssh.NewPublicKey(publicKey)
	if err != nil {
		return ProxyIdentity{}, fmt.Errorf("encode asset proxy public key: %w", err)
	}
	now := i.now()
	expiresAt := now.Add(time.Hour)
	cert := &ssh.Certificate{
		Key: sshPublicKey, CertType: ssh.UserCert, KeyId: agentID,
		ValidPrincipals: []string{a.Name},
		ValidAfter:      uint64(now.Add(-time.Minute).Unix()),
		ValidBefore:     uint64(expiresAt.Unix()),
		Permissions: ssh.Permissions{Extensions: map[string]string{
			certOrgID: orgID, certAgentID: agentID, certAssetID: a.ID,
		}},
	}
	if err := cert.SignCert(rand.Reader, i.ca); err != nil {
		return ProxyIdentity{}, fmt.Errorf("sign asset proxy certificate: %w", err)
	}
	privateDER, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return ProxyIdentity{}, fmt.Errorf("marshal asset proxy private key: %w", err)
	}
	return ProxyIdentity{
		AssetID: a.ID, AssetName: a.Name,
		PrivateKey:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER})),
		Certificate: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(cert))),
		ExpiresAt:   expiresAt,
	}, nil
}

type Proxy struct {
	assets Assets
	client *Client
	config *ssh.ServerConfig
}

func NewProxy(assets Assets, client *Client, encryptionKey []byte) (*Proxy, error) {
	if assets == nil || client == nil || len(encryptionKey) == 0 {
		return nil, errors.New("asset SSH proxy requires assets, client, and an encryption key")
	}
	ca, err := deterministicSigner(encryptionKey, "helix-asset-ssh-ca")
	if err != nil {
		return nil, err
	}
	host, err := deterministicSigner(encryptionKey, "helix-asset-ssh-host")
	if err != nil {
		return nil, err
	}
	p := &Proxy{assets: assets, client: client}
	checker := &ssh.CertChecker{IsUserAuthority: func(key ssh.PublicKey) bool {
		return hmac.Equal(key.Marshal(), ca.PublicKey().Marshal())
	}}
	p.config = &ssh.ServerConfig{PublicKeyCallback: func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
		cert, ok := key.(*ssh.Certificate)
		if !ok {
			return nil, errors.New("Helix asset proxy requires a signed agent certificate")
		}
		if err := checker.CheckCert(meta.User(), cert); err != nil {
			return nil, fmt.Errorf("validate Helix asset proxy certificate: %w", err)
		}
		orgID := cert.Extensions[certOrgID]
		agentID := cert.Extensions[certAgentID]
		assetID := cert.Extensions[certAssetID]
		if orgID == "" || agentID == "" || assetID == "" {
			return nil, errors.New("Helix asset proxy certificate is missing scope")
		}
		a, err := assets.AuthorizeRef(context.Background(), orgID, agentID, assetID)
		if err != nil {
			return nil, fmt.Errorf("authorize asset proxy: %w", err)
		}
		if meta.User() != a.Name {
			return nil, errors.New("SSH username must match the linked asset name")
		}
		return &ssh.Permissions{Extensions: map[string]string{
			certOrgID: orgID, certAgentID: agentID, certAssetID: assetID,
		}}, nil
	}}
	p.config.AddHostKey(host)
	return p, nil
}

func (p *Proxy) Serve(ctx context.Context, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf("listen for asset SSH proxy on %s: %w", address, err)
	}
	return p.serve(ctx, listener)
}

func (p *Proxy) serve(ctx context.Context, listener net.Listener) error {
	defer listener.Close()
	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept asset SSH proxy connection: %w", err)
		}
		go p.handle(ctx, conn)
	}
}

func (p *Proxy) handle(ctx context.Context, raw net.Conn) {
	defer raw.Close()
	conn, channels, requests, err := ssh.NewServerConn(raw, p.config)
	if err != nil {
		return
	}
	defer conn.Close()
	go ssh.DiscardRequests(requests)
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			_ = newChannel.Reject(ssh.UnknownChannelType, "only SSH session channels are supported")
			continue
		}
		go p.proxySession(ctx, conn, newChannel)
	}
}

func (p *Proxy) proxySession(ctx context.Context, conn *ssh.ServerConn, incoming ssh.NewChannel) {
	permissions := conn.Permissions.Extensions
	upstream, err := p.client.DialAuthorized(ctx, permissions[certOrgID], permissions[certAgentID], permissions[certAssetID])
	if err != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, err.Error())
		return
	}
	defer upstream.Close()
	upstreamChannel, upstreamRequests, err := upstream.OpenChannel("session", nil)
	if err != nil {
		_ = incoming.Reject(ssh.ConnectionFailed, "open upstream SSH session: "+err.Error())
		return
	}
	defer upstreamChannel.Close()
	clientChannel, clientRequests, err := incoming.Accept()
	if err != nil {
		return
	}
	defer clientChannel.Close()

	go func() { _, _ = io.Copy(upstreamChannel, clientChannel) }()
	go func() { _, _ = io.Copy(clientChannel.Stderr(), upstreamChannel.Stderr()) }()
	go forwardRequests(clientRequests, upstreamChannel)
	upstreamRequestsDone := make(chan struct{})
	go func() {
		forwardRequests(upstreamRequests, clientChannel)
		close(upstreamRequestsDone)
	}()
	_, _ = io.Copy(clientChannel, upstreamChannel)
	<-upstreamRequestsDone
}

func forwardRequests(requests <-chan *ssh.Request, channel ssh.Channel) {
	for request := range requests {
		ok, err := channel.SendRequest(request.Type, request.WantReply, request.Payload)
		if request.WantReply {
			_ = request.Reply(ok && err == nil, nil)
		}
	}
}

func deterministicSigner(key []byte, purpose string) (ssh.Signer, error) {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(purpose))
	privateKey := ed25519.NewKeyFromSeed(mac.Sum(nil))
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("create %s signer: %w", purpose, err)
	}
	return signer, nil
}

func ParseProxyAddress(value string) (host string, port int, err error) {
	host, rawPort, err := net.SplitHostPort(value)
	if err != nil {
		return "", 0, fmt.Errorf("asset SSH proxy address must be host:port: %w", err)
	}
	port, err = strconv.Atoi(rawPort)
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errors.New("asset SSH proxy port is invalid")
	}
	return host, port, nil
}
