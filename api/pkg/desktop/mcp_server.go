package desktop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer provides MCP tools for desktop interaction.
// It exposes screenshot, clipboard, input, and search tools to AI agents.
type MCPServer struct {
	mcpServer     *server.MCPServer
	sseServer     *server.SSEServer
	screenshotURL string // URL to the local screenshot HTTP endpoint
	helixAPIURL   string // Helix API URL for search endpoints
	helixAPIToken string // Helix API token for authentication
	sessionID     string // Current session ID for context-aware search
	logger        *slog.Logger
}

// MCPConfig holds configuration for the MCP server
type MCPConfig struct {
	// Port for the MCP SSE server (default: 9877)
	Port string
	// ScreenshotURL is the local screenshot endpoint (default: http://localhost:9876/screenshot)
	ScreenshotURL string
	// HelixAPIURL is the Helix API endpoint (from HELIX_API_URL env)
	HelixAPIURL string
	// HelixAPIToken is the API token for Helix (from HELIX_API_TOKEN env)
	HelixAPIToken string
	// SessionID is the current session ID (from ZED_SESSION_ID env)
	SessionID string
}

// NewMCPServer creates a new MCP server for desktop tools.
func NewMCPServer(cfg MCPConfig, logger *slog.Logger) *MCPServer {
	if cfg.Port == "" {
		cfg.Port = "9877"
	}
	if cfg.ScreenshotURL == "" {
		cfg.ScreenshotURL = "http://localhost:9876/screenshot"
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
		screenshotURL: cfg.ScreenshotURL,
		helixAPIURL:   cfg.HelixAPIURL,
		helixAPIToken: cfg.HelixAPIToken,
		sessionID:     cfg.SessionID,
		logger:        logger,
	}

	// Create MCP server
	m.mcpServer = server.NewMCPServer(
		"Helix Desktop",
		"1.0.0",
		server.WithResourceCapabilities(false, false),
		server.WithLogging(),
	)

	// Add screenshot tool
	screenshotTool := mcp.NewTool("take_screenshot",
		mcp.WithDescription("Takes a screenshot of the desktop. Returns a base64-encoded PNG image. Use this to see what's currently displayed on screen."),
	)
	m.mcpServer.AddTool(screenshotTool, m.handleScreenshot)

	// Add save_screenshot tool - saves to file and returns path
	saveScreenshotTool := mcp.NewTool("save_screenshot",
		mcp.WithDescription("Takes a screenshot and saves it to a file. Returns the file path."),
		mcp.WithString("path",
			mcp.Description("Path where to save the screenshot (default: /tmp/screenshot.png)"),
		),
	)
	m.mcpServer.AddTool(saveScreenshotTool, m.handleSaveScreenshot)

	// Add type_text tool - types text via keyboard
	typeTextTool := mcp.NewTool("type_text",
		mcp.WithDescription("Types the given text using keyboard input, as if a user is typing."),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The text to type"),
		),
	)
	m.mcpServer.AddTool(typeTextTool, m.handleTypeText)

	// Add click tool - clicks at screen coordinates
	clickTool := mcp.NewTool("mouse_click",
		mcp.WithDescription("Clicks at the specified screen coordinates."),
		mcp.WithNumber("x",
			mcp.Required(),
			mcp.Description("X coordinate on screen"),
		),
		mcp.WithNumber("y",
			mcp.Required(),
			mcp.Description("Y coordinate on screen"),
		),
		mcp.WithString("button",
			mcp.Description("Mouse button: left, right, or middle (default: left)"),
		),
	)
	m.mcpServer.AddTool(clickTool, m.handleMouseClick)

	// Add get_clipboard tool
	clipboardTool := mcp.NewTool("get_clipboard",
		mcp.WithDescription("Gets the current clipboard text content."),
	)
	m.mcpServer.AddTool(clipboardTool, m.handleGetClipboard)

	// Add set_clipboard tool
	setClipboardTool := mcp.NewTool("set_clipboard",
		mcp.WithDescription("Sets the clipboard text content."),
		mcp.WithString("text",
			mcp.Required(),
			mcp.Description("The text to copy to clipboard"),
		),
	)
	m.mcpServer.AddTool(setClipboardTool, m.handleSetClipboard)

	// Add search_history tool - search through conversation history
	searchHistoryTool := mcp.NewTool("search_history",
		mcp.WithDescription(`Search through your conversation history and previous prompts.
Use this to recall how you solved similar problems before, find commands you used,
or retrieve context from earlier in the conversation that may have been compacted.
Returns matching prompts with their content and metadata.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query - matches against prompt content"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of results to return (default: 20, max: 100)"),
		),
	)
	m.mcpServer.AddTool(searchHistoryTool, m.handleSearchHistory)

	// Add unified_search tool - search across all Helix entities
	unifiedSearchTool := mcp.NewTool("unified_search",
		mcp.WithDescription(`Search across all your Helix data: sessions, tasks, prompts, projects,
repositories, and code. Use this to find relevant context from any previous work.`),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query"),
		),
		mcp.WithString("types",
			mcp.Description("Comma-separated list of types to search: sessions,tasks,prompts,projects,repositories,code,agents,knowledge (default: all)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum results per type (default: 10)"),
		),
	)
	m.mcpServer.AddTool(unifiedSearchTool, m.handleUnifiedSearch)

	// Add get_session_context tool - get current session's interactions
	sessionContextTool := mcp.NewTool("get_session_context",
		mcp.WithDescription(`Get the full conversation history for the current session or a specific session.
Use this to retrieve messages that may have been compacted from your context window.`),
		mcp.WithString("session_id",
			mcp.Description("Session ID to retrieve (default: current session)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of interactions to return (default: 50)"),
		),
	)
	m.mcpServer.AddTool(sessionContextTool, m.handleGetSessionContext)

	// Create SSE server
	m.sseServer = server.NewSSEServer(m.mcpServer,
		server.WithBasePath("/mcp"),
	)

	return m
}

// handleScreenshot takes a screenshot and returns base64 image
func (m *MCPServer) handleScreenshot(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m.logger.Info("taking screenshot via MCP")

	// Fetch screenshot from local endpoint
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(m.screenshotURL)
	if err != nil {
		m.logger.Error("failed to get screenshot", "err", err)
		return mcp.NewToolResultError("failed to capture screenshot: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("screenshot failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read screenshot data: " + err.Error()), nil
	}

	// Encode to base64
	b64 := base64.StdEncoding.EncodeToString(data)

	m.logger.Info("screenshot captured", "size", len(data))

	// Return as image content
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			mcp.ImageContent{
				Type:     "image",
				Data:     b64,
				MIMEType: "image/png",
			},
		},
	}, nil
}

// handleSaveScreenshot saves screenshot to a file
func (m *MCPServer) handleSaveScreenshot(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := "/tmp/screenshot.png"
	if p, err := request.RequireString("path"); err == nil && p != "" {
		path = p
	}

	m.logger.Info("saving screenshot", "path", path)

	// Fetch screenshot from local endpoint
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(m.screenshotURL)
	if err != nil {
		return mcp.NewToolResultError("failed to capture screenshot: " + err.Error()), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return mcp.NewToolResultError(fmt.Sprintf("screenshot failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	// Read image data
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return mcp.NewToolResultError("failed to read screenshot data: " + err.Error()), nil
	}

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return mcp.NewToolResultError("failed to create directory: " + err.Error()), nil
	}

	// Save to file
	if err := os.WriteFile(path, data, 0644); err != nil {
		return mcp.NewToolResultError("failed to save screenshot: " + err.Error()), nil
	}

	m.logger.Info("screenshot saved", "path", path, "size", len(data))
	return mcp.NewToolResultText(fmt.Sprintf("Screenshot saved to %s (%d bytes)", path, len(data))), nil
}

// handleTypeText types text using keyboard input
func (m *MCPServer) handleTypeText(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError("text parameter is required"), nil
	}

	m.logger.Info("typing text via MCP", "length", len(text))

	// Use wtype or ydotool for Wayland text input
	// wtype is preferred for Sway/wlroots compositors
	cmd := exec.CommandContext(ctx, "wtype", text)
	if err := cmd.Run(); err != nil {
		// Fall back to ydotool
		cmd = exec.CommandContext(ctx, "ydotool", "type", "--", text)
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to type text: " + err.Error()), nil
		}
	}

	return mcp.NewToolResultText(fmt.Sprintf("Typed %d characters", len(text))), nil
}

// handleMouseClick clicks at screen coordinates
func (m *MCPServer) handleMouseClick(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	x, err := request.RequireFloat("x")
	if err != nil {
		return mcp.NewToolResultError("x coordinate is required"), nil
	}

	y, err := request.RequireFloat("y")
	if err != nil {
		return mcp.NewToolResultError("y coordinate is required"), nil
	}

	button := "left"
	if b, err := request.RequireString("button"); err == nil && b != "" {
		button = b
	}

	m.logger.Info("mouse click via MCP", "x", x, "y", y, "button", button)

	// Use ydotool for mouse control
	// First move, then click
	moveCmd := exec.CommandContext(ctx, "ydotool", "mousemove", "--absolute", "-x", fmt.Sprintf("%.0f", x), "-y", fmt.Sprintf("%.0f", y))
	if err := moveCmd.Run(); err != nil {
		return mcp.NewToolResultError("failed to move mouse: " + err.Error()), nil
	}

	// Map button name to ydotool button code
	buttonCode := "0" // left
	switch button {
	case "right":
		buttonCode = "1"
	case "middle":
		buttonCode = "2"
	}

	clickCmd := exec.CommandContext(ctx, "ydotool", "click", buttonCode)
	if err := clickCmd.Run(); err != nil {
		return mcp.NewToolResultError("failed to click: " + err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Clicked %s button at (%d, %d)", button, int(x), int(y))), nil
}

// handleGetClipboard gets clipboard content
func (m *MCPServer) handleGetClipboard(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m.logger.Info("getting clipboard via MCP")

	// Use wl-paste for Wayland
	cmd := exec.CommandContext(ctx, "wl-paste")
	output, err := cmd.Output()
	if err != nil {
		return mcp.NewToolResultError("failed to get clipboard: " + err.Error()), nil
	}

	return mcp.NewToolResultText(string(output)), nil
}

// handleSetClipboard sets clipboard content
func (m *MCPServer) handleSetClipboard(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	text, err := request.RequireString("text")
	if err != nil {
		return mcp.NewToolResultError("text parameter is required"), nil
	}

	m.logger.Info("setting clipboard via MCP", "length", len(text))

	// Use wl-copy for Wayland
	cmd := exec.CommandContext(ctx, "wl-copy", text)
	if err := cmd.Run(); err != nil {
		return mcp.NewToolResultError("failed to set clipboard: " + err.Error()), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Copied %d characters to clipboard", len(text))), nil
}

// ServeHTTP serves MCP requests
func (m *MCPServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.sseServer.ServeHTTP(w, r)
}

// Run starts the MCP SSE server
func (m *MCPServer) Run(ctx context.Context, port string) error {
	if port == "" {
		port = "9877"
	}

	httpServer := &http.Server{
		Addr:    ":" + port,
		Handler: m.sseServer,
	}

	m.logger.Info("MCP server starting", "port", port)

	errCh := make(chan error, 1)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		m.logger.Info("MCP server shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// handleSearchHistory searches through prompt history
func (m *MCPServer) handleSearchHistory(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured - HELIX_API_URL and HELIX_API_TOKEN environment variables required"), nil
	}

	limit := 20
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
		if limit > 100 {
			limit = 100
		}
	}

	m.logger.Info("searching prompt history", "query", query, "limit", limit)

	// Build search URL
	searchURL := fmt.Sprintf("%s/api/v1/prompt-history/search?q=%s&limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		url.QueryEscape(query),
		limit,
	)

	// Make request to Helix API
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
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	// Parse response
	var results []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return mcp.NewToolResultError("failed to parse search results: " + err.Error()), nil
	}

	// Format results for display
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d matching prompts:\n\n", len(results)))

	for i, result := range results {
		content, _ := result["content"].(string)
		createdAt, _ := result["created_at"].(string)
		sessionID, _ := result["session_id"].(string)
		pinned, _ := result["pinned"].(bool)

		// Truncate content for display
		if len(content) > 500 {
			content = content[:500] + "..."
		}

		sb.WriteString(fmt.Sprintf("--- Result %d ---\n", i+1))
		if pinned {
			sb.WriteString("📌 PINNED\n")
		}
		sb.WriteString(fmt.Sprintf("Date: %s\n", createdAt))
		if sessionID != "" {
			sb.WriteString(fmt.Sprintf("Session: %s\n", sessionID))
		}
		sb.WriteString(fmt.Sprintf("Content:\n%s\n\n", content))
	}

	if len(results) == 0 {
		sb.WriteString("No matching prompts found. Try different search terms.")
	}

	m.logger.Info("search complete", "results", len(results))
	return mcp.NewToolResultText(sb.String()), nil
}

// handleUnifiedSearch searches across all Helix entities
func (m *MCPServer) handleUnifiedSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, err := request.RequireString("query")
	if err != nil {
		return mcp.NewToolResultError("query parameter is required"), nil
	}

	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured - HELIX_API_URL and HELIX_API_TOKEN environment variables required"), nil
	}

	limit := 10
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
	}

	// Parse types
	types := ""
	if t, err := request.RequireString("types"); err == nil && t != "" {
		types = t
	}

	m.logger.Info("unified search", "query", query, "types", types, "limit", limit)

	// Build search URL
	searchURL := fmt.Sprintf("%s/api/v1/search?q=%s&limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		url.QueryEscape(query),
		limit,
	)
	if types != "" {
		// Add types as separate query params
		for _, t := range strings.Split(types, ",") {
			searchURL += "&types=" + url.QueryEscape(strings.TrimSpace(t))
		}
	}

	// Make request to Helix API
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
		return mcp.NewToolResultError(fmt.Sprintf("search failed: %d - %s", resp.StatusCode, string(body))), nil
	}

	// Parse response
	var searchResp struct {
		Results []struct {
			Type        string            `json:"type"`
			ID          string            `json:"id"`
			Title       string            `json:"title"`
			Description string            `json:"description"`
			URL         string            `json:"url"`
			Metadata    map[string]string `json:"metadata"`
			CreatedAt   string            `json:"created_at"`
		} `json:"results"`
		Total int    `json:"total"`
		Query string `json:"query"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return mcp.NewToolResultError("failed to parse search results: " + err.Error()), nil
	}

	// Format results for display
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d results for '%s':\n\n", searchResp.Total, searchResp.Query))

	// Group by type
	byType := make(map[string][]struct {
		ID          string
		Title       string
		Description string
		CreatedAt   string
	})

	for _, r := range searchResp.Results {
		byType[r.Type] = append(byType[r.Type], struct {
			ID          string
			Title       string
			Description string
			CreatedAt   string
		}{r.ID, r.Title, r.Description, r.CreatedAt})
	}

	typeOrder := []string{"sessions", "tasks", "prompts", "projects", "repositories", "code", "agents", "knowledge"}
	for _, t := range typeOrder {
		items, ok := byType[t]
		if !ok || len(items) == 0 {
			continue
		}

		sb.WriteString(fmt.Sprintf("=== %s (%d) ===\n", strings.ToUpper(t), len(items)))
		for _, item := range items {
			sb.WriteString(fmt.Sprintf("• %s\n", item.Title))
			if item.Description != "" {
				sb.WriteString(fmt.Sprintf("  %s\n", item.Description))
			}
			sb.WriteString(fmt.Sprintf("  ID: %s | Date: %s\n", item.ID, item.CreatedAt))
		}
		sb.WriteString("\n")
	}

	if searchResp.Total == 0 {
		sb.WriteString("No results found. Try different search terms.")
	}

	m.logger.Info("unified search complete", "total", searchResp.Total)
	return mcp.NewToolResultText(sb.String()), nil
}

// handleGetSessionContext retrieves session interactions
func (m *MCPServer) handleGetSessionContext(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if m.helixAPIURL == "" || m.helixAPIToken == "" {
		return mcp.NewToolResultError("Helix API not configured - HELIX_API_URL and HELIX_API_TOKEN environment variables required"), nil
	}

	// Get session ID (use current session if not specified)
	sessionID := m.sessionID
	if s, err := request.RequireString("session_id"); err == nil && s != "" {
		sessionID = s
	}
	if sessionID == "" {
		return mcp.NewToolResultError("no session ID available - either provide session_id parameter or ensure ZED_SESSION_ID is set"), nil
	}

	limit := 50
	if l, err := request.RequireFloat("limit"); err == nil && l > 0 {
		limit = int(l)
		if limit > 200 {
			limit = 200
		}
	}

	m.logger.Info("getting session context", "session_id", sessionID, "limit", limit)

	// Get session with interactions
	sessionURL := fmt.Sprintf("%s/api/v1/sessions/%s?interactions=true&limit=%d",
		strings.TrimSuffix(m.helixAPIURL, "/"),
		sessionID,
		limit,
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
		return mcp.NewToolResultError(fmt.Sprintf("failed to get session: %d - %s", resp.StatusCode, string(body))), nil
	}

	// Parse session response
	var session struct {
		ID           string `json:"id"`
		Name         string `json:"name"`
		Created      string `json:"created"`
		Interactions []struct {
			ID            string `json:"id"`
			Created       string `json:"created"`
			PromptMessage string `json:"prompt_message"`
			AssistantMessage string `json:"assistant_message"`
			State         string `json:"state"`
		} `json:"interactions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return mcp.NewToolResultError("failed to parse session: " + err.Error()), nil
	}

	// Format session context
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Session: %s\n", session.Name))
	sb.WriteString(fmt.Sprintf("ID: %s\n", session.ID))
	sb.WriteString(fmt.Sprintf("Created: %s\n", session.Created))
	sb.WriteString(fmt.Sprintf("Interactions: %d\n\n", len(session.Interactions)))

	for i, interaction := range session.Interactions {
		sb.WriteString(fmt.Sprintf("--- Turn %d (%s) ---\n", i+1, interaction.Created))

		if interaction.PromptMessage != "" {
			prompt := interaction.PromptMessage
			if len(prompt) > 1000 {
				prompt = prompt[:1000] + "... [truncated]"
			}
			sb.WriteString(fmt.Sprintf("USER:\n%s\n\n", prompt))
		}

		if interaction.AssistantMessage != "" {
			assistant := interaction.AssistantMessage
			if len(assistant) > 2000 {
				assistant = assistant[:2000] + "... [truncated]"
			}
			sb.WriteString(fmt.Sprintf("ASSISTANT:\n%s\n\n", assistant))
		}
	}

	if len(session.Interactions) == 0 {
		sb.WriteString("No interactions found in this session.")
	}

	m.logger.Info("session context retrieved", "interactions", len(session.Interactions))
	return mcp.NewToolResultText(sb.String()), nil
}
