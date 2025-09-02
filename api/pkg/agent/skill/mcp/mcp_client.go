package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/data"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/client/transport"
	"github.com/mark3labs/mcp-go/mcp"
)

func newMcpClient(ctx context.Context, cfg *types.AssistantMCP) (*client.Client, error) {
	var t transport.Interface

	switch {
	case strings.HasSuffix(cfg.URL, "sse"):
		sse, err := transport.NewSSE(
			cfg.URL,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create SSE transport: %w", err)
		}
		t = sse
	default:
		httpTransport, err := transport.NewStreamableHTTP(
			cfg.URL,
			transport.WithHTTPHeaders(cfg.Headers),
			// You can add HTTP-specific options here like headers, OAuth, etc.
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create HTTP transport: %w", err)
		}
		defer httpTransport.Close()
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

func InitializeMCPClientSkill(ctx context.Context, cfg *types.AssistantMCP) (*types.ToolMCPClientConfig, error) {
	mcpClient, err := newMcpClient(ctx, cfg)
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
