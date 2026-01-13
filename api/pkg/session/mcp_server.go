package session

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer provides MCP tools for session navigation and history search.
// These tools help AI agents navigate conversation history, find past context,
// and recall how they solved similar problems before.
//
// This is separate from the desktop MCP server because session navigation
// is useful for all agents, not just desktop environments.
type MCPServer struct {
	mcpServer     *server.MCPServer
	sseServer     *server.SSEServer
	helixAPIURL   string // Helix API URL for session endpoints
	helixAPIToken string // Helix API token for authentication
	sessionID     string // Current session ID (most likely to be relevant)
	logger        *slog.Logger
}

// MCPConfig holds configuration for the session MCP server
type MCPConfig struct {
	// Port for the MCP SSE server (default: 9878)
	Port string
	// HelixAPIURL is the Helix API endpoint (from HELIX_API_URL env)
	HelixAPIURL string
	// HelixAPIToken is the API token for Helix (from HELIX_API_TOKEN env)
	HelixAPIToken string
	// SessionID is the current session ID (from ZED_SESSION_ID env)
	SessionID string
}

// NewMCPServer creates a new MCP server for session navigation tools.
func NewMCPServer(cfg MCPConfig, logger *slog.Logger) *MCPServer {
	if cfg.Port == "" {
		cfg.Port = "9878"
	}
	// Get Helix API config from environment if not provided
	if cfg.HelixAPIURL == "" {
		cfg.HelixAPIURL = os.Getenv("HELIX_API_URL")
	}
	if cfg.HelixAPIToken == "" {
		cfg.HelixAPIToken = os.Getenv("HELIX_API_TOKEN")
	}
	if cfg.SessionID == "" {
		cfg.SessionID = os.Getenv("ZED_SESSION_ID")
	}

	m := &MCPServer{
		helixAPIURL:   cfg.HelixAPIURL,
		helixAPIToken: cfg.HelixAPIToken,
		sessionID:     cfg.SessionID,
		logger:        logger,
	}

	// Create MCP server
	m.mcpServer = server.NewMCPServer(
		"Helix Session",
		"1.0.0",
		server.WithResourceCapabilities(false, false),
		server.WithLogging(),
	)

	// =======================================================================
	// LEVEL 1: Current Session (most likely relevant)
	// =======================================================================

	// current_session - Quick overview of the current session
	currentSessionTool := mcp.NewTool("current_session",
		mcp.WithDescription(`Get an overview of the current session - the most likely source of relevant context.
Returns:
- Session ID and current title
- Recent title changes (topic evolution)
- Compact TOC of last 10 turns
- Total turn count

This is the best starting point when you need to recall something from this conversation.
Use session_toc for the full TOC, or get_turn/get_turns for specific content.`),
	)
	m.mcpServer.AddTool(currentSessionTool, m.handleCurrentSession)

	// =======================================================================
	// LEVEL 2: Session Overview (TOC, title history)
	// =======================================================================

	// session_toc - Get table of contents for a session
	sessionTOCTool := mcp.NewTool("session_toc",
		mcp.WithDescription(`Get the table of contents for a session - a numbered list of one-line summaries.
Each entry has a turn number you can use with get_turn or get_turns to retrieve content.

Use this to:
- Understand what topics were discussed
- Find specific turns to retrieve
- Navigate through long conversations`),
		mcp.WithString("session_id",
			mcp.Description("Session ID (default: current session)"),
		),
	)
	m.mcpServer.AddTool(sessionTOCTool, m.handleSessionTOC)

	// session_title_history - See how session topic evolved
	titleHistoryTool := mcp.NewTool("session_title_history",
		mcp.WithDescription(`Get the title history for a session - see how the topic evolved over time.
Each entry shows when the title changed and links to the interaction that triggered it.

Use this to:
- Understand what topics were covered
- Jump to specific conversation phases
- See the evolution of the work being done`),
		mcp.WithString("session_id",
			mcp.Description("Session ID (default: current session)"),
		),
	)
	m.mcpServer.AddTool(titleHistoryTool, m.handleTitleHistory)

	// =======================================================================
	// LEVEL 3: Search & Filter
	// =======================================================================

	// search_session - Search within a session
	searchSessionTool := mcp.NewTool("search_session",
		mcp.WithDescription(`Search for content within a session's conversation history.
Returns matching turns with their summaries and turn numbers.
More focused than search_all_sessions - searches within a single session.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query to find in prompts, responses, or summaries"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (default: current session)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results to return (default: 10)"),
		),
	)
	m.mcpServer.AddTool(searchSessionTool, m.handleSearchSession)

	// search_all_sessions - Cross-session search
	searchAllSessionsTool := mcp.NewTool("search_all_sessions",
		mcp.WithDescription(`Search for keywords across ALL your sessions.
Returns matching sessions with hit summaries and turn numbers.

Use this to:
- Find where you discussed something before
- Recall solutions from past sessions
- Then use session_toc and get_turn to drill down`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum sessions to return (default: 10)"),
		),
	)
	m.mcpServer.AddTool(searchAllSessionsTool, m.handleSearchAllSessions)

	// list_sessions - Discover recent sessions
	listSessionsTool := mcp.NewTool("list_sessions",
		mcp.WithDescription(`List your recent sessions with their current titles.
Use this to discover what sessions exist and pick one to explore.`),
		mcp.WithNumber("limit",
			mcp.Description("Maximum sessions to return (default: 20, max: 50)"),
		),
	)
	m.mcpServer.AddTool(listSessionsTool, m.handleListSessions)

	// =======================================================================
	// LEVEL 4: Content Retrieval
	// =======================================================================

	// get_turn - Get a specific turn by number
	getTurnTool := mcp.NewTool("get_turn",
		mcp.WithDescription(`Get a specific conversation turn by its number (from session_toc).
Returns the full prompt and response, plus brief summaries of prev/next turns for context.`),
		mcp.WithNumber("turn",
			mcp.Required(),
			mcp.Description("Turn number (1-indexed)"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (default: current session)"),
		),
	)
	m.mcpServer.AddTool(getTurnTool, m.handleGetTurn)

	// get_turns - Get a range of turns
	getTurnsTool := mcp.NewTool("get_turns",
		mcp.WithDescription(`Get a range of conversation turns (e.g., turns 5-10).
More efficient than calling get_turn multiple times.
Maximum range is 20 turns.`),
		mcp.WithNumber("from",
			mcp.Required(),
			mcp.Description("Starting turn number (1-indexed, inclusive)"),
		),
		mcp.WithNumber("to",
			mcp.Required(),
			mcp.Description("Ending turn number (1-indexed, inclusive)"),
		),
		mcp.WithString("session_id",
			mcp.Description("Session ID (default: current session)"),
		),
	)
	m.mcpServer.AddTool(getTurnsTool, m.handleGetTurns)

	// get_interaction - Get by interaction ID
	getInteractionTool := mcp.NewTool("get_interaction",
		mcp.WithDescription(`Get a specific interaction by its ID.
Use this when you have an interaction ID (e.g., from title_history) and want to jump directly to it.`),
		mcp.WithString("interaction_id",
			mcp.Required(),
			mcp.Description("The interaction ID to retrieve"),
		),
	)
	m.mcpServer.AddTool(getInteractionTool, m.handleGetInteraction)

	// Create SSE server
	m.sseServer = server.NewSSEServer(m.mcpServer,
		server.WithBasePath("/mcp"),
	)

	return m
}

// ServeHTTP serves MCP requests
func (m *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.sseServer.ServeHTTP(w, r)
}

// Run starts the MCP SSE server
func (m *MCPServer) Run(ctx context.Context, port string) error {
	if port == "" {
		port = "9878"
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: m.sseServer,
	}

	m.logger.Info("Session MCP server starting", "port", port)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		m.logger.Info("Session MCP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// =========================================================================
// Handler implementations
// =========================================================================

func (m *MCPServer) handleCurrentSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured - HELIX_API_URL and HELIX_API_TOKEN required"), nil
	}

	if m.sessionID == "" {
		return mcp.NewToolResultError("no current session - ZED_SESSION_ID not set"), nil
	}

	m.logger.Info("getting current session overview", "session_id", m.sessionID)

	sessionURL := fmt.Sprintf("%s/api/v1/sessions/%s",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		m.sessionID,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", sessionURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var session struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Created      string `json:"created"`
		Interactions []struct {
			ID              string `json:"id"`
			PromptMessage   string `json:"prompt_message"`
			ResponseMessage string `json:"response_message"`
			Summary         string `json:"summary"`
		} `json:"interactions"`
		Metadata struct {
			TitleHistory []struct {
				Title     string `json:"title"`
				ChangedAt string `json:"changed_at"`
				Turn      int    `json:"turn"`
			} `json:"title_history"`
		} `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return mcp.NewToolResultError("failed to parse session: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString("=== CURRENT SESSION ===\n\n")
	sb.WriteString(fmt.Sprintf("Title: %s\n", session.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n", session.ID))
	sb.WriteString(fmt.Sprintf("Created: %s\n", session.Created))
	sb.WriteString(fmt.Sprintf("Total turns: %d\n\n", len(session.Interactions)))

	// Show recent title changes
	if len(session.Metadata.TitleHistory) > 0 {
		sb.WriteString("Topic evolution (recent titles):\n")
		maxHistory := 3
		if len(session.Metadata.TitleHistory) < maxHistory {
			maxHistory = len(session.Metadata.TitleHistory)
		}
		for i := 0; i < maxHistory; i++ {
			entry := session.Metadata.TitleHistory[i]
			sb.WriteString(fmt.Sprintf("  Turn %d: \"%s\"\n", entry.Turn, entry.Title))
		}
		sb.WriteString("\n")
	}

	// Show last 10 turns as compact TOC
	sb.WriteString("Recent turns (last 10):\n")
	startIdx := 0
	if len(session.Interactions) > 10 {
		startIdx = len(session.Interactions) - 10
	}
	for i := startIdx; i < len(session.Interactions); i++ {
		interaction := session.Interactions[i]
		turn := i + 1
		summary := interaction.Summary
		if summary == "" && interaction.PromptMessage != "" {
			lines := strings.Split(interaction.PromptMessage, "\n")
			summary = lines[0]
			if len(summary) > 80 {
				summary = summary[:77] + "..."
			}
		}
		sb.WriteString(fmt.Sprintf("  %d. %s\n", turn, summary))
	}

	sb.WriteString("\nUse session_toc() for full TOC, get_turn(turn=N) for specific content.")
	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleSessionTOC(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available"), nil
	}

	m.logger.Info("getting session TOC", "session_id", sessionID)

	tocURL := fmt.Sprintf("%s/api/v1/sessions/%s/toc",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", tocURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get TOC: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var toc struct {
		SessionID   string `json:"session_id"`
		SessionName string `json:"session_name"`
		TotalTurns  int    `json:"total_turns"`
		Formatted   string `json:"formatted"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&toc); err != nil {
		return mcp.NewToolResultError("failed to parse TOC: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session: %s\n", toc.SessionName))
	sb.WriteString(fmt.Sprintf("Total turns: %d\n\n", toc.TotalTurns))
	sb.WriteString("Table of Contents:\n")
	sb.WriteString(toc.Formatted)
	sb.WriteString("\n\nUse get_turn(turn=N) to retrieve specific content, get_turns(from=X, to=Y) for ranges.")

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleTitleHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available"), nil
	}

	m.logger.Info("getting title history", "session_id", sessionID)

	sessionURL := fmt.Sprintf("%s/api/v1/sessions/%s",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", sessionURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get session: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var session struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Metadata struct {
			TitleHistory []struct {
				Title         string `json:"title"`
				ChangedAt     string `json:"changed_at"`
				Turn          int    `json:"turn"`
				InteractionID string `json:"interaction_id"`
			} `json:"title_history"`
		} `json:"config"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return mcp.NewToolResultError("failed to parse session: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session: %s\n", session.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n\n", session.ID))

	if len(session.Metadata.TitleHistory) == 0 {
		sb.WriteString("No title history yet - the session title hasn't changed.\n")
	} else {
		sb.WriteString("Title evolution (newest first):\n\n")
		for i, entry := range session.Metadata.TitleHistory {
			sb.WriteString(fmt.Sprintf("%d. \"%s\"\n", i+1, entry.Title))
			sb.WriteString(fmt.Sprintf("   Changed at turn %d | %s\n", entry.Turn, entry.ChangedAt))
			sb.WriteString(fmt.Sprintf("   Jump to: get_interaction(interaction_id=\"%s\")\n\n", entry.InteractionID))
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleSearchSession(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	query, err := request.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available"), nil
	}

	limit := 10
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
		if limit > 50 {
			limit = 50
		}
	}

	m.logger.Info("searching session", "session_id", sessionID, "query", query)

	searchURL := fmt.Sprintf("%s/api/v1/sessions/%s/search?q=%s&limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
		url.QueryEscape(query),
		limit,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to search: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var results struct {
		SessionName string `json:"session_name"`
		TotalTurns  int    `json:"total_turns"`
		Entries     []struct {
			Turn    int    `json:"turn"`
			Summary string `json:"summary"`
		} `json:"entries"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return mcp.NewToolResultError("failed to parse results: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for \"%s\" in session %s:\n\n", query, results.SessionName))

	if len(results.Entries) == 0 {
		sb.WriteString("No matching turns found.")
	} else {
		for _, entry := range results.Entries {
			sb.WriteString(fmt.Sprintf("Turn %d: %s\n", entry.Turn, entry.Summary))
		}
		sb.WriteString(fmt.Sprintf("\nFound %d matches out of %d turns.", len(results.Entries), results.TotalTurns))
		sb.WriteString("\n\nUse get_turn(turn=N) to retrieve specific content.")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleSearchAllSessions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	query, err := request.RequireString("query")
	if err != nil || query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}

	limit := 10
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
		if limit > 30 {
			limit = 30
		}
	}

	m.logger.Info("searching all sessions", "query", query)

	searchURL := fmt.Sprintf("%s/api/v1/sessions/search?q=%s&limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		url.QueryEscape(query),
		limit,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to search: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var results struct {
		Sessions []struct {
			SessionID   string `json:"session_id"`
			SessionName string `json:"session_name"`
			Matches     []struct {
				Turn    int    `json:"turn"`
				Summary string `json:"summary"`
			} `json:"matches"`
		} `json:"sessions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return mcp.NewToolResultError("failed to parse results: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Cross-session search for \"%s\":\n", query))
	sb.WriteString(fmt.Sprintf("Found matches in %d sessions\n\n", len(results.Sessions)))

	for _, s := range results.Sessions {
		currentMarker := ""
		if s.SessionID == m.sessionID {
			currentMarker = " [CURRENT]"
		}
		sb.WriteString(fmt.Sprintf("Session: %s%s\n", s.SessionName, currentMarker))
		sb.WriteString(fmt.Sprintf("ID: %s\n", s.SessionID))
		for _, match := range s.Matches {
			sb.WriteString(fmt.Sprintf("  Turn %d: %s\n", match.Turn, match.Summary))
		}
		sb.WriteString("\n")
	}

	if len(results.Sessions) == 0 {
		sb.WriteString("No matches found. Try different search terms.")
	} else {
		sb.WriteString("Use session_toc(session_id=...) for full TOC, get_turn(turn=N, session_id=...) for details.")
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleListSessions(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	limit := 20
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
		if limit > 50 {
			limit = 50
		}
	}

	m.logger.Info("listing sessions", "limit", limit)

	listURL := fmt.Sprintf("%s/api/v1/sessions?limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		limit,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", listURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to list sessions: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var sessions []struct {
		ID      string `json:"session_id"`
		Name    string `json:"name"`
		Created string `json:"created"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&sessions); err != nil {
		return mcp.NewToolResultError("failed to parse sessions: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Recent sessions (%d):\n\n", len(sessions)))

	for i, s := range sessions {
		currentMarker := ""
		if s.ID == m.sessionID {
			currentMarker = " [CURRENT]"
		}
		title := s.Name
		if title == "" {
			title = "(untitled)"
		}
		sb.WriteString(fmt.Sprintf("%d. %s%s\n   ID: %s | Created: %s\n\n", i+1, title, currentMarker, s.ID, s.Created))
	}

	sb.WriteString("Use session_toc(session_id=...) to see the table of contents for any session.")
	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleGetTurn(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	turn, err := request.RequireFloat("turn")
	if err != nil || turn < 1 {
		return mcp.NewToolResultError("turn number is required and must be positive"), nil
	}

	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available"), nil
	}

	m.logger.Info("getting turn", "session_id", sessionID, "turn", int(turn))

	turnURL := fmt.Sprintf("%s/api/v1/sessions/%s/turns/%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
		int(turn),
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", turnURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get turn: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var turnData struct {
		Turn        int `json:"turn"`
		Interaction struct {
			PromptMessage   string `json:"prompt_message"`
			ResponseMessage string `json:"response_message"`
			Summary         string `json:"summary"`
		} `json:"interaction"`
		Previous *struct {
			Turn    int    `json:"turn"`
			Summary string `json:"summary"`
		} `json:"previous"`
		Next *struct {
			Turn    int    `json:"turn"`
			Summary string `json:"summary"`
		} `json:"next"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&turnData); err != nil {
		return mcp.NewToolResultError("failed to parse turn: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Turn %d ===\n\n", turnData.Turn))

	if turnData.Previous != nil {
		sb.WriteString(fmt.Sprintf("Previous turn %d: %s\n\n", turnData.Previous.Turn, turnData.Previous.Summary))
	}

	if turnData.Interaction.PromptMessage != "" {
		sb.WriteString("USER:\n")
		sb.WriteString(turnData.Interaction.PromptMessage)
		sb.WriteString("\n\n")
	}

	if turnData.Interaction.ResponseMessage != "" {
		sb.WriteString("ASSISTANT:\n")
		sb.WriteString(turnData.Interaction.ResponseMessage)
		sb.WriteString("\n\n")
	}

	if turnData.Next != nil {
		sb.WriteString(fmt.Sprintf("Next turn %d: %s\n", turnData.Next.Turn, turnData.Next.Summary))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleGetTurns(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	fromTurn, err := request.RequireFloat("from")
	if err != nil || fromTurn < 1 {
		return mcp.NewToolResultError("'from' is required and must be positive"), nil
	}

	toTurn, err := request.RequireFloat("to")
	if err != nil || toTurn < 1 {
		return mcp.NewToolResultError("'to' is required and must be positive"), nil
	}

	if toTurn < fromTurn {
		return mcp.NewToolResultError("'to' must be >= 'from'"), nil
	}
	if toTurn-fromTurn > 20 {
		return mcp.NewToolResultError("maximum range is 20 turns"), nil
	}

	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available"), nil
	}

	m.logger.Info("getting turns", "session_id", sessionID, "from", int(fromTurn), "to", int(toTurn))

	turnsURL := fmt.Sprintf("%s/api/v1/sessions/%s/turns?from=%d&to=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
		int(fromTurn),
		int(toTurn),
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", turnsURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get turns: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var turnsData struct {
		SessionName string `json:"session_name"`
		Turns       []struct {
			Turn            int    `json:"turn"`
			PromptMessage   string `json:"prompt_message"`
			ResponseMessage string `json:"response_message"`
			Summary         string `json:"summary"`
		} `json:"turns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&turnsData); err != nil {
		return mcp.NewToolResultError("failed to parse turns: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Turns %d-%d from \"%s\":\n\n", int(fromTurn), int(toTurn), turnsData.SessionName))

	for _, t := range turnsData.Turns {
		sb.WriteString(fmt.Sprintf("=== Turn %d ===\n", t.Turn))
		if t.Summary != "" {
			sb.WriteString(fmt.Sprintf("Summary: %s\n\n", t.Summary))
		}
		if t.PromptMessage != "" {
			sb.WriteString("USER:\n")
			sb.WriteString(t.PromptMessage)
			sb.WriteString("\n\n")
		}
		if t.ResponseMessage != "" {
			sb.WriteString("ASSISTANT:\n")
			sb.WriteString(t.ResponseMessage)
			sb.WriteString("\n\n")
		}
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (m *MCPServer) handleGetInteraction(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured"), nil
	}

	interactionID, err := request.RequireString("interaction_id")
	if err != nil || interactionID == "" {
		return mcp.NewToolResultError("interaction_id is required"), nil
	}

	m.logger.Info("getting interaction", "interaction_id", interactionID)

	interactionURL := fmt.Sprintf("%s/api/v1/interactions/%s",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		interactionID,
	)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", interactionURL, nil)
	if err != nil {
		return mcp.NewToolResultError("failed to create request: " + err.Error()), nil
	}
	req.Header.Set("Authorization", "Bearer "+m.helixAPIToken)

	resp, err := client.Do(req)
	if err != nil {
		return mcp.NewToolResultError("failed to get interaction: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	var interaction struct {
		ID              string `json:"id"`
		SessionID       string `json:"session_id"`
		Created         string `json:"created"`
		PromptMessage   string `json:"prompt_message"`
		ResponseMessage string `json:"response_message"`
		Summary         string `json:"summary"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&interaction); err != nil {
		return mcp.NewToolResultError("failed to parse interaction: " + err.Error()), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Interaction %s\n", interaction.ID))
	sb.WriteString(fmt.Sprintf("Session: %s\n", interaction.SessionID))
	sb.WriteString(fmt.Sprintf("Created: %s\n", interaction.Created))
	if interaction.Summary != "" {
		sb.WriteString(fmt.Sprintf("Summary: %s\n", interaction.Summary))
	}
	sb.WriteString("\n")

	if interaction.PromptMessage != "" {
		sb.WriteString("USER:\n")
		sb.WriteString(interaction.PromptMessage)
		sb.WriteString("\n\n")
	}

	if interaction.ResponseMessage != "" {
		sb.WriteString("ASSISTANT:\n")
		sb.WriteString(interaction.ResponseMessage)
		sb.WriteString("\n")
	}

	return mcp.NewToolResultText(sb.String()), nil
}
