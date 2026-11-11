package mcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"maps"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/oauth"
	"github.com/helixml/helix/api/pkg/types"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

//go:generate mockgen -source $GOFILE -destination mcp_client_mocks.go -package $GOPACKAGE

type ClientGetter interface {
	NewClient(ctx context.Context, meta agent.Meta, oauthManager *oauth.Manager, cfg *types.AssistantMCP) (Client, error)
}

type Client interface {
	ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error)
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

type DefaultClientGetter struct {
	TLSSkipVerify bool

	// Outbound, when set, restricts the addresses MCP server URLs may reach.
	// MCP server URLs are user-supplied and connected to from the API server.
	Outbound *netguard.Policy
	// TrustedHosts are host:port values exempt from Outbound: the Helix API's
	// own address, which built-in MCP servers (e.g. Kodit) are served from.
	TrustedHosts []string

	httpClientOnce sync.Once
	httpClient     *http.Client
}

// client returns the HTTP client shared by every MCP connection this getter
// creates (shared so per-connection clients need no explicit closing).
func (d *DefaultClientGetter) client() *http.Client {
	d.httpClientOnce.Do(func() {
		open := http.DefaultTransport.(*http.Transport).Clone()
		if d.TLSSkipVerify {
			open.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		if d.Outbound == nil {
			d.httpClient = &http.Client{Transport: open}
			return
		}
		guarded := open.Clone()
		d.Outbound.GuardTransport(guarded)
		trusted := make(map[string]bool, len(d.TrustedHosts))
		for _, h := range d.TrustedHosts {
			trusted[strings.ToLower(h)] = true
		}
		d.httpClient = &http.Client{Transport: &trustedHostTransport{open: open, guarded: guarded, trusted: trusted}}
	})
	return d.httpClient
}

// trustedHostTransport sends requests for trusted hosts through open and all
// others through guarded. It is consulted per request, so redirects away from
// a trusted host are guarded again.
type trustedHostTransport struct {
	open, guarded http.RoundTripper
	trusted       map[string]bool
}

func (t *trustedHostTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.trusted[strings.ToLower(canonicalHostPort(req.URL))] {
		return t.open.RoundTrip(req)
	}
	return t.guarded.RoundTrip(req)
}

// canonicalHostPort returns u's host:port, filling in the scheme's default port.
func canonicalHostPort(u *url.URL) string {
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

// NewDefaultClientGetter returns a getter for user-configured MCP servers,
// restricted by policy except for serverURL (the Helix API's own address).
func NewDefaultClientGetter(tlsSkipVerify bool, policy netguard.Policy, serverURL string) *DefaultClientGetter {
	getter := &DefaultClientGetter{TLSSkipVerify: tlsSkipVerify, Outbound: &policy}
	if u, err := url.Parse(serverURL); err == nil && u.Hostname() != "" {
		getter.TrustedHosts = []string{canonicalHostPort(u)}
	}
	return getter
}

func (d *DefaultClientGetter) NewClient(ctx context.Context, meta agent.Meta, oauthManager *oauth.Manager, cfg *types.AssistantMCP) (Client, error) {
	var t transport.Interface

	// Copy the headers without changing the original map
	headers := make(map[string]string)

	maps.Copy(headers, cfg.Headers)

	// Tell the MCP server which session, user and app this call is for.
	//
	// WHY THIS MATTERS. An MCP server that needs to act on behalf of the end user
	// otherwise has to take their identity from a TOOL ARGUMENT, which the model
	// controls — and a model that reads untrusted input (an uploaded CV, a web
	// page) can be talked into sending someone else's id. These headers are set
	// here, by Helix, from the session it is actually running; nothing the model
	// emits can change them, so a server can treat them as trustworthy in a way
	// it can never treat an argument.
	//
	// Set AFTER the operator's own headers on purpose: a configured header must
	// not be able to spoof the acting session.
	if meta.SessionID != "" {
		headers["X-Helix-Session-Id"] = meta.SessionID
	}
	if meta.UserID != "" {
		headers["X-Helix-User-Id"] = meta.UserID
	}
	if meta.AppID != "" {
		headers["X-Helix-App-Id"] = meta.AppID
	}

	if cfg.OAuthProvider != "" && headers["Authorization"] == "" {
		// Get the token with required scopes
		token, err := oauthManager.GetTokenForTool(ctx, meta.UserID, cfg.OAuthProvider, cfg.OAuthScopes)
		if err != nil {
			return nil, fmt.Errorf("failed to get token for MCP server: %w", err)
		}
		// Set bearer token
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}

	httpClient := d.client()

	switch {
	case cfg.Transport == "sse" || strings.HasSuffix(cfg.URL, "sse"):
		// Use SSE transport if explicitly configured or URL ends with "sse"
		sse, err := transport.NewSSE(
			cfg.URL,
			transport.WithHeaders(headers),
			transport.WithHTTPClient(httpClient),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSE transport: %w", err)
		}
		t = sse
	default:
		httpTransport, err := transport.NewStreamableHTTP(
			cfg.URL,
			transport.WithHTTPHeaders(headers),
			transport.WithHTTPTimeout(60*time.Second),
			// Passing default client so that we don't need to close it
			transport.WithHTTPBasicClient(httpClient),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP transport: %w", err)
		}
		// defer httpTransport.Close()
		t = httpTransport
	}

	mcpClient := client.NewClient(
		t,
	)

	err := mcpClient.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to start client: %w", err)
	}

	// Initialize the MCP session
	initRequest := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			Capabilities:    mcp.ClientCapabilities{},
			ClientInfo: mcp.Implementation{
				Name:    "helix-http-client",
				Version: data.GetHelixVersion(),
			},
		},
	}

	_, err = mcpClient.Initialize(ctx, initRequest)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize MCP session: %w", err)
	}

	return mcpClient, nil
}

func InitializeMCPClientSkill(ctx context.Context, clientGetter ClientGetter, meta agent.Meta, oauthManager *oauth.Manager, cfg *types.AssistantMCP) (*types.ToolMCPClientConfig, error) {
	mcpClient, err := clientGetter.NewClient(ctx, meta, oauthManager, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create MCP client: %w", err)
	}

	// List tools, server description
	toolsResp, err := mcpClient.ListTools(ctx, mcp.ListToolsRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to list tools: %w", err)
	}

	return &types.ToolMCPClientConfig{
		Name:        cfg.Name,
		Description: cfg.Description,
		Tools:       toolsResp.Tools,
	}, nil
}
