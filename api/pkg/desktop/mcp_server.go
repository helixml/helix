package desktop

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// MCPServer provides MCP tools for desktop interaction.
// It exposes screenshot, clipboard, and input tools to AI agents.
type MCPServer struct {
	mcpServer     *server.MCPServer
	sseServer     *server.SSEServer
	screenshotURL string // URL to the local screenshot HTTP endpoint
	logger        *slog.Logger
}

// MCPConfig holds configuration for the MCP server
type MCPConfig struct {
	// Port for the MCP SSE server (default: 9877)
	Port string
	// ScreenshotURL is the local screenshot endpoint (default: http://localhost:9876/screenshot)
	ScreenshotURL string
}

// NewMCPServer creates a new MCP server for desktop tools.
func NewMCPServer(cfg MCPConfig, logger *slog.Logger) *MCPServer {
	if cfg.Port == "" {
		cfg.Port = "9877"
	}
	if cfg.ScreenshotURL == "" {
		cfg.ScreenshotURL = "http://localhost:9876/screenshot"
	}

	m := &MCPServer{
		screenshotURL: cfg.ScreenshotURL,
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
