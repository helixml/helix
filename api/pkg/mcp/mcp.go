package mcp

/*

Helix MCP Server

Ref: https://modelcontextprotocol.io/quickstart/server

## Core MCP Concepts

MCP servers can provide three main types of capabilities:
- Resources: File-like data that can be read by clients (like API responses or file contents)
- Tools: Functions that can be called by the LLM (with user approval)
- Prompts: Pre-written templates that help users accomplish specific tasks

Tools:
- MCP inspector: https://github.com/modelcontextprotocol/inspector
  To run it:
	```
	npx @modelcontextprotocol/inspector
	```

*/
import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func NewServer(
	store store.Store,
	r rag.RAG,
	ctrl *controller.Controller,
) *Server {
	return &Server{
		store:      store,
		rag:        r,
		controller: ctrl,
		planner:    ctrl.ToolsPlanner,
		newRagClient: func(settings *types.RAGSettings) rag.RAG {
			return r
		},
	}
}

const (
	InternalPort = 21000
)

type Server struct {
	store      store.Store
	rag        rag.RAG
	controller *controller.Controller
	planner    tools.Planner

	newRagClient func(settings *types.RAGSettings) rag.RAG
}

// TODO: load this dynamically from the app
// TODO: add multi-tenancy, so we can have multiple apps on the same server

func (s *Server) Run(ctx context.Context) error {
	// Create MCP server

	srv := server.NewMCPServer(
		"Demo 🚀",
		"1.0.0",
	)

	assistant, err := s.loadAssistant(ctx, &types.User{
		ID:    testUserID,
		AppID: testAppID,
	})
	if err != nil {
		return fmt.Errorf("failed to load assistant: %w", err)
	}

	// for _, tool := range assistant.Tools {
	// 	switch tool.ToolType {
	// 	case types.ToolTypeAPI:

	// 	}

	// 	mcpTool := mcp.NewTool(tool.Name,
	// 		mcp.WithDescription(tool.Description),
	// 	)

	// 	srv.AddTool(mcpTool, helloHandler)
	// }

	// Add tool
	tool := mcp.NewTool("hello_world",
		mcp.WithDescription("Say hello to someone"),
		mcp.WithString("name",
			mcp.Required(),
			mcp.Description("Name of the person to greet"),
		),
	)

	// Add tool handler
	// TODO: dynamically remove tools from the server
	srv.AddTool(tool, helloHandler)

	mcpServer := server.NewSSEServer(srv, "")

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_ = mcpServer.Shutdown(shutdownCtx)
	}()

	err = mcpServer.Start(fmt.Sprintf(":%d", InternalPort))
	if err != nil {
		return fmt.Errorf("failed to run MCP server: %w", err)
	}

	return nil
}

const (
	testAppID  = "app_01jfsqaq25xytqc1ep8hrt7php"
	testUserID = "191517eb-4dd3-4e71-8370-40d419f1c854"
)

func (s *Server) configureTools(server *server.MCPServer) {

}

func (s *Server) loadAssistant(ctx context.Context, user *types.User) (*types.AssistantConfig, error) {
	return s.controller.LoadAssistant(ctx, user, &controller.ChatCompletionOptions{
		AppID: testAppID,
	})
}

func helloHandler(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {

	name, ok := request.Params.Arguments["name"].(string)
	if !ok {
		return mcp.NewToolResultError("name must be a string"), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Hello, %s!", name)), nil
}

type helixMCPTool struct {
	appID   string
	tool    *mcp.Tool
	handler func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// TODO: load gptscript, zapier tools as well
func getModelContextProtocolTools(app *types.AssistantConfig) ([]*helixMCPTool, error) {
	var mcpTools []*helixMCPTool

	for _, tool := range app.Tools {
		switch tool.ToolType {
		case types.ToolTypeAPI:
			// Each API tool has a list of actions, adding them separately
			for _, action := range tool.Config.API.Actions {
				mcpTool := mcp.NewTool(action.Name,
					mcp.WithDescription(action.Description),
				)

				mcpTools = append(mcpTools, &helixMCPTool{
					appID:   app.ID,
					tool:    &mcpTool,
					handler: helloHandler,
				})
			}
		case types.ToolTypeGPTScript:
			mcpTool := mcp.NewTool(tool.Name,
				mcp.WithDescription(tool.Description),
			)

			mcpTools = append(mcpTools, &helixMCPTool{
				appID:   app.ID,
				tool:    &mcpTool,
				handler: helloHandler,
			})
		case types.ToolTypeZapier:
			mcpTool := mcp.NewTool(tool.Name,
				mcp.WithDescription(tool.Description),
			)
			mcpTools = append(mcpTools, &helixMCPTool{
				appID:   app.ID,
				tool:    &mcpTool,
				handler: helloHandler,
			})
		}

	}

	return mcpTools, nil
}
