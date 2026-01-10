package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// SessionMCPBackend provides session navigation MCP tools via HTTP
// This allows AI agents to navigate their own conversation history.
type SessionMCPBackend struct {
	store      store.Store
	mcpServer  *server.MCPServer
	httpServer *server.StreamableHTTPServer
}

// NewSessionMCPBackend creates a new session MCP backend
func NewSessionMCPBackend(s store.Store) *SessionMCPBackend {
	backend := &SessionMCPBackend{
		store: s,
	}

	// Create MCP server
	backend.mcpServer = server.NewMCPServer(
		"Helix Session",
		"1.0.0",
		server.WithResourceCapabilities(false, false),
		server.WithLogging(),
	)

	// Add current_session tool
	currentSessionTool := mcp.NewTool("current_session",
		mcp.WithDescription("Get quick overview of the current session including name, turn count, and recent activity."),
	)
	backend.mcpServer.AddTool(currentSessionTool, backend.handleCurrentSession)

	// Add session_toc tool
	sessionTocTool := mcp.NewTool("session_toc",
		mcp.WithDescription("Get the table of contents for a session - numbered list of all turns with one-line summaries. Great for finding specific past discussions."),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session from query param)"),
		),
	)
	backend.mcpServer.AddTool(sessionTocTool, backend.handleSessionTOC)

	// Add get_turn tool
	getTurnTool := mcp.NewTool("get_turn",
		mcp.WithDescription("Get the full content of a specific turn (interaction) by turn number."),
		mcp.WithNumber("turn",
			mcp.Required(),
			mcp.Description("Turn number (1-indexed)"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(getTurnTool, backend.handleGetTurn)

	// Add session_title_history tool
	titleHistoryTool := mcp.NewTool("session_title_history",
		mcp.WithDescription("See how the session title evolved over time - shows topic changes and which turn triggered each title."),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(titleHistoryTool, backend.handleTitleHistory)

	// Add search_session tool
	searchSessionTool := mcp.NewTool("search_session",
		mcp.WithDescription("Search within a session's interactions for specific terms or topics."),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (defaults to current session)"),
		),
	)
	backend.mcpServer.AddTool(searchSessionTool, backend.handleSearchSession)

	// Create Streamable HTTP server for direct POST support
	// Use stateless mode so each request is independent (no session tracking required)
	backend.httpServer = server.NewStreamableHTTPServer(backend.mcpServer,
		server.WithStateLess(true),
	)

	return backend
}

// ServeHTTP implements MCPBackend interface
func (b *SessionMCPBackend) ServeHTTP(w http.ResponseWriter, r *http.Request, user *types.User) {
	// Store user and session_id in context for tool handlers
	ctx := r.Context()
	ctx = context.WithValue(ctx, "user", user)
	ctx = context.WithValue(ctx, "session_id", r.URL.Query().Get("session_id"))
	r = r.WithContext(ctx)

	b.httpServer.ServeHTTP(w, r)
}

// Helper to get session ID from context or request
func (b *SessionMCPBackend) getSessionID(ctx context.Context, requestedID string) string {
	if requestedID != "" {
		return requestedID
	}
	if id, ok := ctx.Value("session_id").(string); ok && id != "" {
		return id
	}
	return ""
}

// handleCurrentSession returns quick overview of current session
func (b *SessionMCPBackend) handleCurrentSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	// Count interactions
	_, total, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   1,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to count interactions: " + err.Error()), nil
	}

	result := map[string]interface{}{
		"session_id":   session.ID,
		"name":         session.Name,
		"total_turns":  total,
		"created":      session.Created,
		"updated":      session.Updated,
		"title_changes": len(session.Metadata.TitleHistory),
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleSessionTOC returns the table of contents
func (b *SessionMCPBackend) handleSessionTOC(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	// Get all interactions
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   200, // Reasonable limit
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	// Build TOC
	var toc []map[string]interface{}
	for i, interaction := range interactions {
		entry := map[string]interface{}{
			"turn":    i + 1,
			"id":      interaction.ID,
			"summary": interaction.Summary,
		}
		if interaction.Summary == "" && interaction.PromptMessage != "" {
			// Fallback to first 80 chars of prompt
			summary := interaction.PromptMessage
			if len(summary) > 80 {
				summary = summary[:80] + "..."
			}
			entry["summary"] = summary
		}
		toc = append(toc, entry)
	}

	result := map[string]interface{}{
		"session_id":   sessionID,
		"session_name": session.Name,
		"total_turns":  len(interactions),
		"entries":      toc,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleGetTurn returns a specific turn's content
func (b *SessionMCPBackend) handleGetTurn(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	turn, err := request.RequireFloat("turn")
	if err != nil || turn < 1 {
		return mcp.NewToolResultError("turn number is required (1 or greater)"), nil
	}

	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	// Get interactions and find the one at this turn
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   int(turn) + 1,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	turnIndex := int(turn) - 1
	if turnIndex >= len(interactions) {
		return mcp.NewToolResultError("turn " + strconv.Itoa(int(turn)) + " not found (session has " + strconv.Itoa(len(interactions)) + " turns)"), nil
	}

	interaction := interactions[turnIndex]
	result := map[string]interface{}{
		"turn":     int(turn),
		"id":       interaction.ID,
		"prompt":   interaction.PromptMessage,
		"response": interaction.ResponseMessage,
		"summary":  interaction.Summary,
		"created":  interaction.Created,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleTitleHistory returns the session's title evolution
func (b *SessionMCPBackend) handleTitleHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}

	result := map[string]interface{}{
		"session_id":    sessionID,
		"current_title": session.Name,
		"history":       session.Metadata.TitleHistory,
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// handleSearchSession searches within session interactions
func (b *SessionMCPBackend) handleSearchSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	requestedID, _ := request.RequireString("session_id")
	sessionID := b.getSessionID(ctx, requestedID)
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	// Get all interactions and search
	interactions, _, err := b.store.ListInteractions(ctx, &types.ListInteractionsQuery{
		SessionID: sessionID,
		PerPage:   200,
	})
	if err != nil {
		return mcp.NewToolResultError("failed to list interactions: " + err.Error()), nil
	}

	// Simple text search
	var matches []map[string]interface{}
	for i, interaction := range interactions {
		if containsIgnoreCase(interaction.PromptMessage, query) ||
			containsIgnoreCase(interaction.ResponseMessage, query) ||
			containsIgnoreCase(interaction.Summary, query) {
			matches = append(matches, map[string]interface{}{
				"turn":    i + 1,
				"id":      interaction.ID,
				"summary": interaction.Summary,
			})
		}
	}

	result := map[string]interface{}{
		"session_id": sessionID,
		"query":      query,
		"matches":    matches,
		"total":      len(matches),
	}

	jsonBytes, _ := json.MarshalIndent(result, "", "  ")
	return mcp.NewToolResultText(string(jsonBytes)), nil
}

// containsIgnoreCase performs case-insensitive substring search
func containsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
