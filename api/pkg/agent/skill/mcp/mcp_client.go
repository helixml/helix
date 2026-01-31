package mcp

import (
	"context"
	"crypto/tls"
	"fmt"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/agent"
	"github.com/helixml/helix/api/pkg/data"
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
}

func (d *DefaultClientGetter) NewClient(ctx context.Context, meta agent.Meta, oauthManager *oauth.Manager, cfg *types.AssistantMCP) (Client, error) {
	var t transport.Interface

	// Copy the headers without changing the original map
	headers := make(map[string]string)

	maps.Copy(headers, cfg.Headers)

	if cfg.OAuthProvider != "" && headers["Authorization"] == "" {
		// Get the token with required scopes
		token, err := oauthManager.GetTokenForTool(ctx, meta.UserID, cfg.OAuthProvider, cfg.OAuthScopes)
		if err != nil {
			return nil, fmt.Errorf("failed to get token for MCP server: %w", err)
		}
		// Set bearer token
		headers["Authorization"] = fmt.Sprintf("Bearer %s", token)
	}

	httpClient := http.DefaultClient
	if d.TLSSkipVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

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
