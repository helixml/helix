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
	"github.com/sashabaranov/go-openai"
)

type MCPClientTool struct { //nolint:revive
	tool *types.ToolMCPClientConfig
}

func (t *MCPClientTool) Name() string {
	return t.tool.Name
}

func (t *MCPClientTool) Description() string {
	return t.tool.Description
}

func (t *MCPClientTool) Execute(ctx context.Context, args map[string]interface{}) (string, error) {
	// TODO: initialize client, call it and return the result
	return "", nil
}

func (t *MCPClientTool) Icon() string {
	return ""
}

func (t *MCPClientTool) String() string {
	return "MCPClient"
}

func (t *MCPClientTool) StatusMessage() string {
	return "MCPClient"
}

func (t *MCPClientTool) OpenAI() []openai.Tool {
	return []openai.Tool{
		{
			// TODO: dynamically generate the function definition based on the MCP client
			Type: openai.ToolTypeFunction,
			Function: &openai.FunctionDefinition{
				Name:        "mcp_",
				Description: "mcp_description",
				// TODOL: parameters
			},
		},
	}
}

func InitializeMCPClientSkill(ctx context.Context, cfg *types.AssistantMCP) (*types.ToolMCPClientConfig, error) {
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
