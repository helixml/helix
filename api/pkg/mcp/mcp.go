package mcp

/*

Helix MCP Server

Ref: https://modelcontextprotocol.io/quickstart/server

## Core MCP Concepts

MCP servers can provide three main types of capabilities:
- Resources: File-like data that can be read by clients (like API responses or file contents)
- Tools: Functions that can be called by the LLM (with user approval)
- Prompts: Pre-written templates that help users accomplish specific tasks

*/
import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewServer() *Server {
	return &Server{}
}

const (
	InternalPort = 21000
	// Should match the API prefix in the server
	BaseURL = "/api/v1/mcp"
)

type Server struct {
}

func (s *Server) Run(ctx context.Context) error {
	// Create MCP server
	srv := server.NewMCPServer(
		"Demo 🚀",
		"1.0.0",
	)

	// Add tool
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	// Add tool handler
	srv.AddTool(tool, helloHandler)

	server.NewStdioServer(srv)

	mcpServer := server.NewSSEServer(srv, BaseURL)

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = mcpServer.Shutdown(shutdownCtx)
	}()

	err := mcpServer.Start(fmt.Sprintf(":%d", InternalPort))
	if err != nil {
		return fmt.Errorf("failed to run MCP server: %w", err)
	}

	return nil
}

func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name, ok := request.Params.Arguments["name"].(string)
	if !ok {
		return mcp.NewToolResultError("name must be a string"), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}
