package server

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
	"github.com/sourcegraph/conc/pool"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/rag"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/tools"
	"github.com/helixml/helix/api/pkg/types"
)

// HelixMCPBackend implements MCPBackend for Helix native tools (APIs, Knowledge, Zapier)
// This serves the same tools as the CLI MCP server, but over HTTP for external agents
type HelixMCPBackend struct {
	store      store.Store
	controller *controller.Controller

	// Cache of SSE servers per app ID (each app has different tools)
	servers   map[string]*helixAppMCPServer
	serversMu sync.RWMutex
}

// helixAppMCPServer holds the MCP server for a specific app
type helixAppMCPServer struct {
	sseServer *server.SSEServer
	appID     string
	createdAt time.Time
}

// NewHelixMCPBackend creates a new Helix MCP backend
func NewHelixMCPBackend(store store.Store, ctrl *controller.Controller) *HelixMCPBackend {
	return &HelixMCPBackend{
		store:      store,
		controller: ctrl,
		servers:    make(map[string]*helixAppMCPServer),
	}
}

// ServeHTTP implements MCPBackend
func (b *HelixMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	// Get app_id from query parameter
	appID := r.URL.Query().Get("app_id")
	if appID == "" {
		http.Error(w, "app_id query parameter is required", http.StatusBadRequest)
		return
	}

	// Get or create SSE server for this app
	sseServer, err := b.getOrCreateServer(r.Context(), user, appID)
	if err != nil {
		log.Error().Err(err).Str("app_id", appID).Msg("failed to create MCP server for app")
		http.Error(w, "failed to initialize MCP server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Get the path suffix after /api/v1/mcp/helix
	vars := mux.Vars(r)
	pathSuffix := vars["path"]

	log.Debug().
		Str("user_id", user.ID).
		Str("app_id", appID).
		Str("method", r.Method).
		Str("path_suffix", pathSuffix).
		Msg("handling Helix MCP request")

	// The SSE server handles routing based on path:
	// - /sse endpoint for SSE connections
	// - /message endpoint for POST requests
	sseServer.ServeHTTP(w, r)
}

// getOrCreateServer gets or creates an SSE server for the given app
func (b *HelixMCPBackend) getOrCreateServer(ctx context.Context, user *types.User, appID string) (*server.SSEServer, error) {
	// Check cache first (with TTL check)
	cacheKey := fmt.Sprintf("%s:%s", user.ID, appID)
	cacheTTL := 5 * time.Minute

	b.serversMu.RLock()
	if appServer, ok := b.servers[cacheKey]; ok {
		// Check if cache is still valid
		if time.Since(appServer.createdAt) < cacheTTL {
			b.serversMu.RUnlock()
			return appServer.sseServer, nil
		}
	}
	b.serversMu.RUnlock()

	// Create new server
	b.serversMu.Lock()
	defer b.serversMu.Unlock()

	// Double-check after acquiring write lock
	if appServer, ok := b.servers[cacheKey]; ok {
		if time.Since(appServer.createdAt) < cacheTTL {
			return appServer.sseServer, nil
		}
		// Cache expired, shutdown old server
		if err := appServer.sseServer.Shutdown(ctx); err != nil {
			log.Warn().Err(err).Str("app_id", appID).Msg("failed to shutdown old MCP server")
		}
	}

	// Get app from database
	app, err := b.store.GetApp(ctx, appID)
	if err != nil {
		return nil, fmt.Errorf("failed to get app: %w", err)
	}

	// Verify user has access to the app
	if app.Owner != user.ID {
		// TODO: Check access grants for shared apps
		return nil, fmt.Errorf("access denied: user does not own this app")
	}

	// Create MCP server
	mcpServer := server.NewMCPServer(
		"Helix ML",
		"1.0.0",
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Add tools from app config
	if len(app.Config.Helix.Assistants) > 0 {
		assistant := &app.Config.Helix.Assistants[0]
		if err := b.addToolsFromAssistant(ctx, mcpServer, user, app, assistant); err != nil {
			log.Warn().Err(err).Str("app_id", appID).Msg("failed to add tools from assistant")
		}
	}

	// Create SSE server with base path matching gateway route
	sseServer := server.NewSSEServer(mcpServer,
		server.WithBasePath("/api/v1/mcp/helix"),
	)

	// Cache the server
	b.servers[cacheKey] = &helixAppMCPServer{
		sseServer: sseServer,
		appID:     appID,
		createdAt: time.Now(),
	}

	log.Info().
		Str("app_id", appID).
		Str("user_id", user.ID).
		Msg("created Helix MCP server for app")

	return sseServer, nil
}

// addToolsFromAssistant adds MCP tools from the assistant config
// This mirrors the logic from api/pkg/cli/mcp/mcp_proxy.go
func (b *HelixMCPBackend) addToolsFromAssistant(ctx context.Context, mcpServer *server.MCPServer, user *types.User, app *types.App, assistant *types.AssistantConfig) error {
	// Add API tools
	for _, apiConfig := range assistant.APIs {
		tool, err := store.ConvertAPIToTool(apiConfig)
		if err != nil {
			log.Error().Err(err).Msg("failed to convert API config to tool")
			continue
		}

		// Add each action as a separate MCP tool
		for _, action := range tool.Config.API.Actions {
			params, err := tools.GetParametersFromSchema(tool.Config.API.Schema, action.Name)
			if err != nil {
				log.Error().Err(err).Str("tool", tool.Name).Str("action", action.Name).
					Msg("failed to get parameters from schema")
				continue
			}

			var mcpParams []mcp.ToolOption
			mcpParams = append(mcpParams, mcp.WithDescription(action.Description))

			for _, param := range params {
				if param.Required {
					mcpParams = append(mcpParams, mcp.WithString(param.Name,
						mcp.Required(),
						mcp.Description(param.Description),
					))
				} else {
					mcpParams = append(mcpParams, mcp.WithString(param.Name,
						mcp.Description(param.Description),
					))
				}
			}

			mcpTool := mcp.NewTool(action.Name, mcpParams...)
			mcpServer.AddTool(mcpTool, b.createAPIToolHandler(app.ID, tool, action.Name))

			log.Debug().
				Str("app_id", app.ID).
				Str("action", action.Name).
				Msg("added API tool to MCP server")
		}
	}

	// Add Zapier tools
	for _, zapier := range assistant.Zapier {
		mcpTool := mcp.NewTool(zapier.Name,
			mcp.WithDescription(zapier.Description),
		)
		mcpServer.AddTool(mcpTool, b.createZapierToolHandler(app.ID, zapier))

		log.Debug().
			Str("app_id", app.ID).
			Str("zapier", zapier.Name).
			Msg("added Zapier tool to MCP server")
	}

	// Add Knowledge tools
	knowledges, err := b.store.ListKnowledge(ctx, &store.ListKnowledgeQuery{
		AppID: app.ID,
	})
	if err != nil {
		log.Warn().Err(err).Str("app_id", app.ID).Msg("failed to list knowledge sources")
	} else {
		for _, knowledge := range knowledges {
			description := `Performs a search using the Helix knowledge base, ideal for finding information on a specific topic.`
			if knowledge.Description != "" {
				description += fmt.Sprintf(" This tool contains information on: %s", knowledge.Description)
			}

			mcpTool := mcp.NewTool(knowledge.Name,
				mcp.WithDescription(description),
				mcp.WithString("query",
					mcp.Required(),
					mcp.Description("For query use concise, main keywords as the engine is performing both semantic and full text search"),
				),
			)
			mcpServer.AddTool(mcpTool, b.createKnowledgeToolHandler(app.ID, knowledge))

			log.Debug().
				Str("app_id", app.ID).
				Str("knowledge", knowledge.Name).
				Msg("added Knowledge tool to MCP server")
		}
	}

	return nil
}

// createAPIToolHandler creates a handler for API tool calls
// This mirrors api/pkg/cli/mcp/mcp_proxy.go getAPIToolHandler
func (b *HelixMCPBackend) createAPIToolHandler(appID string, tool *types.Tool, actionName string) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		log.Info().
			Str("app_id", appID).
			Str("tool", tool.Name).
			Str("action", actionName).
			Msg("executing API tool")

		// Build parameters from request
		params := make(map[string]interface{})
		for k, v := range request.GetArguments() {
			params[k] = v
		}

		// Execute the API action using the controller's ToolsPlanner
		req := &types.RunAPIActionRequest{
			Action:     actionName,
			Parameters: params,
			Tool:       tool,
		}

		resp, err := b.controller.ToolsPlanner.RunAPIActionWithParameters(ctx, req)
		if err != nil {
			log.Error().Err(err).Str("action", actionName).Msg("API action failed")
			return mcp.NewToolResultError("API action failed: " + err.Error()), nil
		}

		return mcp.NewToolResultText(resp.Response), nil
	}
}

// createZapierToolHandler creates a handler for Zapier tool calls
func (b *HelixMCPBackend) createZapierToolHandler(appID string, zapier types.AssistantZapier) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		// TODO: Implement Zapier tool execution
		// The CLI version also has this as TODO
		log.Warn().Str("app_id", appID).Str("zapier", zapier.Name).Msg("Zapier tool not yet implemented")
		return mcp.NewToolResultError("Zapier integration not yet implemented"), nil
	}
}

// createKnowledgeToolHandler creates a handler for Knowledge tool calls
// This mirrors api/pkg/cli/mcp/mcp_proxy.go getKnowledgeToolHandler
func (b *HelixMCPBackend) createKnowledgeToolHandler(appID string, knowledge *types.Knowledge) func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		query, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError("query parameter is required"), nil
		}

		log.Info().
			Str("app_id", appID).
			Str("knowledge_id", knowledge.ID).
			Str("query", query).
			Msg("searching knowledge")

		// Get RAG client for this knowledge source
		client, err := b.controller.GetRagClient(ctx, knowledge)
		if err != nil {
			log.Error().Err(err).Str("knowledge_id", knowledge.ID).Msg("failed to get RAG client")
			return mcp.NewToolResultError("failed to initialize knowledge search: " + err.Error()), nil
		}

		// Parse filter actions from query
		filterActions := rag.ParseFilterActions(query)
		filterDocumentIDs := make([]string, 0)
		for _, filterAction := range filterActions {
			filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
		}

		// Determine pipeline type
		pipeline := types.TextPipeline
		if knowledge.RAGSettings.EnableVision {
			pipeline = types.VisionPipeline
		}

		// Execute RAG query
		results, err := client.Query(ctx, &types.SessionRAGQuery{
			Prompt:            query,
			DataEntityID:      knowledge.GetDataEntityID(),
			DistanceThreshold: knowledge.RAGSettings.Threshold,
			DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
			MaxResults:        knowledge.RAGSettings.ResultsCount,
			DocumentIDList:    filterDocumentIDs,
			Pipeline:          pipeline,
		})
		if err != nil {
			log.Error().Err(err).Str("knowledge_id", knowledge.ID).Msg("knowledge search failed")
			return mcp.NewToolResultError("knowledge search failed: " + err.Error()), nil
		}

		return mcp.NewToolResultText(formatRAGResults(results)), nil
	}
}

// formatRAGResults formats RAG search results as text
func formatRAGResults(results []*types.SessionRAGResult) string {
	if len(results) == 0 {
		return "No results found"
	}

	var buf bytes.Buffer
	for _, r := range results {
		buf.WriteString(fmt.Sprintf("Source: %s\nContent: %s\n\n", r.Source, r.Content))
	}

	return buf.String()
}

// searchKnowledgesInParallel searches multiple knowledge sources in parallel
// This is used when no specific knowledge_id is provided
func (b *HelixMCPBackend) searchKnowledgesInParallel(ctx context.Context, appID, query string, knowledges []*types.Knowledge) []*types.KnowledgeSearchResult {
	var (
		results   []*types.KnowledgeSearchResult
		resultsMu sync.Mutex
	)

	p := pool.New().
		WithMaxGoroutines(20).
		WithErrors()

	for _, knowledge := range knowledges {
		knowledge := knowledge

		client, err := b.controller.GetRagClient(ctx, knowledge)
		if err != nil {
			log.Error().Err(err).Str("knowledge_id", knowledge.ID).Msg("failed to get RAG client")
			continue
		}

		p.Go(func() error {
			start := time.Now()

			filterActions := rag.ParseFilterActions(query)
			filterDocumentIDs := make([]string, 0)
			for _, filterAction := range filterActions {
				filterDocumentIDs = append(filterDocumentIDs, rag.ParseDocID(filterAction))
			}

			pipeline := types.TextPipeline
			if knowledge.RAGSettings.EnableVision {
				pipeline = types.VisionPipeline
			}

			resp, err := client.Query(ctx, &types.SessionRAGQuery{
				Prompt:            query,
				DataEntityID:      knowledge.GetDataEntityID(),
				DistanceThreshold: knowledge.RAGSettings.Threshold,
				DistanceFunction:  knowledge.RAGSettings.DistanceFunction,
				MaxResults:        knowledge.RAGSettings.ResultsCount,
				DocumentIDList:    filterDocumentIDs,
				Pipeline:          pipeline,
			})
			if err != nil {
				log.Error().Err(err).Str("knowledge_id", knowledge.ID).Msg("RAG query failed")
				return nil // Don't fail the whole search
			}

			resultsMu.Lock()
			results = append(results, &types.KnowledgeSearchResult{
				Knowledge:  knowledge,
				Results:    resp,
				DurationMs: time.Since(start).Milliseconds(),
			})
			resultsMu.Unlock()

			return nil
		})
	}

	_ = p.Wait()
	return results
}
