package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/controller"
	"github.com/helixml/helix/api/pkg/notification"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/helixml/helix/api/pkg/visualization"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/rs/zerolog/log"
)

// SessionMCPBackend provides session navigation MCP tools via HTTP
// This allows AI agents to navigate their own conversation history.
type SessionMCPBackend struct {
	store      store.Store
	notifier   notification.Notifier
	controller *controller.Controller
	mcpServer  *server.MCPServer
	httpServer *server.StreamableHTTPServer
}

// NewSessionMCPBackend creates a new session MCP backend. The controller is
// used by the html_render visualization tool to store pages in the filestore;
// it may be nil in tests that do not exercise that tool.
func NewSessionMCPBackend(s store.Store, notifier notification.Notifier, ctrl *controller.Controller) *SessionMCPBackend {
	backend := &SessionMCPBackend{
		store:      s,
		notifier:   notifier,
		controller: ctrl,
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

	// Add task_completed tool. It is session-scoped and only accepts recurring
	// job sessions with an active trigger execution.
	taskCompletedTool := mcp.NewTool("task_completed",
		mcp.WithDescription("Mark the current recurring task execution complete. Call this exactly once, only after all requested work is finished."),
		mcp.WithString("summary",
			mcp.Description("Optional concise summary of the completed work"),
		),
	)
	backend.mcpServer.AddTool(taskCompletedTool, backend.handleTaskCompleted)

	// Add html_render tool. Publishes a self-contained HTML page (chart, table,
	// diagram, collage, mockup) inline in the current thread, above the agent's
	// final text reply. It reaches spec tasks and ordinary chat automatically
	// (the base Zed config always wires helix-session), and org bots only when
	// their instance profile keeps helix-session (minimal-by-default strips it).
	// See design/2026-10-06-agent-visualizations.md for the availability matrix.
	htmlRenderTool := mcp.NewTool(visualization.ToolName,
		mcp.WithDescription("Show a finished HTML page (chart, table, diagram, collage, mockup) inline in this thread, above your final text reply; call it before writing that reply. The reader already sees the page, so the reply should not announce it, say where it is, or restate it: add only what the page doesn't say. Write one self-contained document with inline <style> and <script>; remote http(s) URLs such as a CDN chart library load as-is. The frame fits the page's height automatically. "+visualization.LayoutGuide+" "+visualization.ThemeGuide),
		mcp.WithString("html",
			mcp.Required(),
			mcp.Description("A complete, self-contained HTML document."),
		),
		mcp.WithString("title",
			mcp.Required(),
			mcp.Description("Short name for the page."),
		),
		mcp.WithNumber("height",
			mcp.Description(fmt.Sprintf("Optional frame height in CSS pixels, %d-%d. The frame auto-fits the page, so this is only an initial hint; omit it to let the page set its own height.", visualization.MinHeight, visualization.MaxHeight)),
		),
	)
	backend.mcpServer.AddTool(htmlRenderTool, backend.handleHTMLRender)

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

func (b *SessionMCPBackend) handleTaskCompleted(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	user, ok := ctx.Value("user").(*types.User)
	if !ok || user == nil || session.Owner != user.ID {
		return mcp.NewToolResultError("not authorized to complete this task"), nil
	}
	if session.Metadata.SessionRole != "job" {
		return mcp.NewToolResultError("current session is not a recurring task"), nil
	}

	summary, _ := request.RequireString("summary")
	if summary == "" {
		summary = "Task completed"
	}
	execution, err := b.store.FinishTriggerExecution(ctx, sessionID, types.TriggerExecutionStatusSuccess, summary)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return mcp.NewToolResultError("no running recurring task execution exists for this session"), nil
		}
		return mcp.NewToolResultError("failed to complete recurring task: " + err.Error()), nil
	}

	if err := b.notifyTaskCompleted(ctx, session, execution, summary); err != nil {
		log.Error().Err(err).
			Str("session_id", session.ID).
			Str("execution_id", execution.ID).
			Msg("recurring task completed but notification failed")
	}

	return mcp.NewToolResultText(fmt.Sprintf("Recurring task execution %s marked complete.", execution.ID)), nil
}

// handleHTMLRender publishes an agent-authored HTML page as a visualization
// stored in the session's filestore folder, rendered inline by the frontend.
func (b *SessionMCPBackend) handleHTMLRender(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sessionID := b.getSessionID(ctx, "")
	if sessionID == "" {
		return mcp.NewToolResultError("session_id is required"), nil
	}
	if b.controller == nil {
		return mcp.NewToolResultError("visualizations are not available on this server"), nil
	}

	html, err := request.RequireString("html")
	if err != nil {
		return mcp.NewToolResultError("html is required"), nil
	}
	if strings.TrimSpace(html) == "" {
		return mcp.NewToolResultError("html must not be empty"), nil
	}
	if len(html) > visualization.MaxHTMLBytes {
		return mcp.NewToolResultError(fmt.Sprintf("html is %d bytes; the limit is %d", len(html), visualization.MaxHTMLBytes)), nil
	}

	title := visualization.CleanTitle(request.GetString("title", ""))
	// A height of 0 means "let the page set its own height"; the client fits the
	// frame to the page's reported content height either way.
	height := 0
	if raw := request.GetFloat("height", 0); raw > 0 {
		height = visualization.ClampHeight(int(raw))
	}

	session, err := b.store.GetSession(ctx, sessionID)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	user, ok := ctx.Value("user").(*types.User)
	if !ok || user == nil || session.Owner != user.ID {
		return mcp.NewToolResultError("not authorized to publish to this session"), nil
	}

	vizID := "viz_" + system.GenerateID()
	prepared := visualization.InjectBootstrap(html)
	if _, err := b.controller.FilestoreVisualizationWrite(ctx, session.Owner, sessionID, vizID, strings.NewReader(prepared)); err != nil {
		return mcp.NewToolResultError("failed to store visualization: " + err.Error()), nil
	}

	ref := visualization.Reference{ID: vizID, Title: title, Height: height}
	payload, err := json.Marshal(ref)
	if err != nil {
		return mcp.NewToolResultError("failed to encode visualization reference: " + err.Error()), nil
	}

	// The marker lets the frontend find the reference no matter how a harness
	// wraps tool output. The human-readable line reminds the agent not to
	// restate the page in its reply.
	result := fmt.Sprintf(
		"%s %s\nShown to the reader above your reply. Don't mention or describe the page; reply with only what it doesn't already say.",
		visualization.ResultMarker, string(payload),
	)
	return mcp.NewToolResultText(result), nil
}

func (b *SessionMCPBackend) notifyTaskCompleted(ctx context.Context, session *types.Session, execution *types.TriggerExecution, summary string) error {
	if b.notifier == nil {
		return nil
	}
	triggerConfig, err := b.store.GetTriggerConfiguration(ctx, &store.GetTriggerConfigurationQuery{ID: execution.TriggerConfigurationID})
	if err != nil {
		return fmt.Errorf("load trigger notification configuration: %w", err)
	}
	if triggerConfig.Trigger.Cron == nil {
		return fmt.Errorf("trigger %s has no cron notification configuration", triggerConfig.ID)
	}
	return b.notifier.Notify(ctx, &types.Notification{
		Event:          types.EventCronTriggerComplete,
		Session:        session,
		Message:        summary,
		RenderMarkdown: true,
		Emails:         triggerConfig.Trigger.Cron.Emails,
		CallbackURL:    triggerConfig.Trigger.Cron.CallbackURL,
	})
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
		"session_id":    session.ID,
		"name":          session.Name,
		"total_turns":   total,
		"created":       session.Created,
		"updated":       session.Updated,
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
