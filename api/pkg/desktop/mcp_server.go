package desktop

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

	// =========================================================================
	// Window Management Tools
	// =========================================================================

	// list_windows - Get all open windows
	listWindowsTool := mcp.NewTool("list_windows",
		mcp.WithDescription("Lists all open windows with their IDs, titles, and workspace. Use window IDs with other window tools."),
	)
	m.mcpServer.AddTool(listWindowsTool, m.handleListWindows)

	// focus_window - Focus a specific window
	focusWindowTool := mcp.NewTool("focus_window",
		mcp.WithDescription("Focus a specific window by ID or title."),
		mcp.WithString("window_id",
			mcp.Description("Window ID from list_windows (preferred)"),
		),
		mcp.WithString("title",
			mcp.Description("Window title to search for (partial match)"),
		),
	)
	m.mcpServer.AddTool(focusWindowTool, m.handleFocusWindow)

	// maximize_window - Maximize a window
	maximizeWindowTool := mcp.NewTool("maximize_window",
		mcp.WithDescription("Maximize a window. If no window specified, maximizes the focused window."),
		mcp.WithString("window_id",
			mcp.Description("Window ID from list_windows"),
		),
		mcp.WithBoolean("fullscreen",
			mcp.Description("Toggle fullscreen instead of maximize (default: false)"),
		),
	)
	m.mcpServer.AddTool(maximizeWindowTool, m.handleMaximizeWindow)

	// tile_window - Tile a window left or right
	tileWindowTool := mcp.NewTool("tile_window",
		mcp.WithDescription("Tile a window to the left or right half of the screen."),
		mcp.WithString("direction",
			mcp.Required(),
			mcp.Description("Direction to tile: 'left' or 'right'"),
		),
		mcp.WithString("window_id",
			mcp.Description("Window ID from list_windows (default: focused window)"),
		),
	)
	m.mcpServer.AddTool(tileWindowTool, m.handleTileWindow)

	// move_to_workspace - Move a window to a specific workspace/desktop
	moveToWorkspaceTool := mcp.NewTool("move_to_workspace",
		mcp.WithDescription("Move a window to a specific workspace/desktop number."),
		mcp.WithNumber("workspace",
			mcp.Required(),
			mcp.Description("Workspace number to move to (1-indexed)"),
		),
		mcp.WithString("window_id",
			mcp.Description("Window ID from list_windows (default: focused window)"),
		),
	)
	m.mcpServer.AddTool(moveToWorkspaceTool, m.handleMoveToWorkspace)

	// switch_to_workspace - Switch to a specific workspace/desktop
	switchToWorkspaceTool := mcp.NewTool("switch_to_workspace",
		mcp.WithDescription("Switch to a specific workspace/desktop number."),
		mcp.WithNumber("workspace",
			mcp.Required(),
			mcp.Description("Workspace number to switch to (1-indexed)"),
		),
	)
	m.mcpServer.AddTool(switchToWorkspaceTool, m.handleSwitchToWorkspace)

	// get_workspaces - List available workspaces
	getWorkspacesTool := mcp.NewTool("get_workspaces",
		mcp.WithDescription("List all workspaces/desktops with their names and which is currently focused."),
	)
	m.mcpServer.AddTool(getWorkspacesTool, m.handleGetWorkspaces)

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

// =========================================================================
// Window Management Handlers
// =========================================================================

// detectDesktopEnvironment determines if we're running Sway, GNOME, or other
func (m *MCPServer) detectDesktopEnvironment() string {
	// Check for Sway
	if _, err := exec.LookPath("swaymsg"); err == nil {
		// Verify we're actually in a Sway session
		cmd := exec.Command("swaymsg", "-t", "get_version")
		if err := cmd.Run(); err == nil {
			return "sway"
		}
	}
	// Check for GNOME
	if os.Getenv("XDG_CURRENT_DESKTOP") == "GNOME" || os.Getenv("DESKTOP_SESSION") == "gnome" {
		return "gnome"
	}
	// Default to sway (most common in our containers)
	return "sway"
}

// handleListWindows lists all open windows
func (m *MCPServer) handleListWindows(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m.logger.Info("listing windows via MCP")

	de := m.detectDesktopEnvironment()

	var output []byte
	var err error

	switch de {
	case "sway":
		// Use swaymsg to get window tree as JSON
		cmd := exec.CommandContext(ctx, "swaymsg", "-t", "get_tree")
		output, err = cmd.Output()
		if err != nil {
			return mcp.NewToolResultError("failed to get window list: " + err.Error()), nil
		}
		// Parse and format the output
		formatted := formatSwayWindowList(output)
		return mcp.NewToolResultText(formatted), nil

	case "gnome":
		// Use wmctrl for GNOME/X11 or gdbus for Wayland GNOME
		cmd := exec.CommandContext(ctx, "wmctrl", "-l", "-p")
		output, err = cmd.Output()
		if err != nil {
			// Try qdbus as fallback
			cmd = exec.CommandContext(ctx, "gdbus", "call", "--session",
				"--dest=org.gnome.Shell",
				"--object-path=/org/gnome/Shell",
				"--method=org.gnome.Shell.Eval",
				"global.get_window_actors().map(a=>a.meta_window.get_title()).join('\\n')")
			output, err = cmd.Output()
			if err != nil {
				return mcp.NewToolResultError("failed to get window list: " + err.Error()), nil
			}
		}
		return mcp.NewToolResultText(string(output)), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleFocusWindow focuses a specific window
func (m *MCPServer) handleFocusWindow(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	windowID, _ := request.RequireString("window_id")
	title, _ := request.RequireString("title")

	if windowID == "" && title == "" {
		return mcp.NewToolResultError("window_id or title is required"), nil
	}

	m.logger.Info("focusing window via MCP", "window_id", windowID, "title", title)

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		var cmd *exec.Cmd
		if windowID != "" {
			cmd = exec.CommandContext(ctx, "swaymsg", fmt.Sprintf("[con_id=%s]", windowID), "focus")
		} else {
			cmd = exec.CommandContext(ctx, "swaymsg", fmt.Sprintf("[title=%q]", title), "focus")
		}
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to focus window: " + err.Error()), nil
		}
		return mcp.NewToolResultText("Window focused"), nil

	case "gnome":
		if windowID != "" {
			cmd := exec.CommandContext(ctx, "wmctrl", "-i", "-a", windowID)
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to focus window: " + err.Error()), nil
			}
		} else {
			cmd := exec.CommandContext(ctx, "wmctrl", "-a", title)
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to focus window: " + err.Error()), nil
			}
		}
		return mcp.NewToolResultText("Window focused"), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleMaximizeWindow maximizes a window
func (m *MCPServer) handleMaximizeWindow(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	windowID, _ := request.RequireString("window_id")
	fullscreen, _ := request.RequireBool("fullscreen")

	m.logger.Info("maximizing window via MCP", "window_id", windowID, "fullscreen", fullscreen)

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		var cmd *exec.Cmd
		action := "fullscreen enable"
		if !fullscreen {
			// In Sway, we toggle fullscreen or use floating + resize to simulate maximize
			action = "fullscreen toggle"
		}
		if windowID != "" {
			cmd = exec.CommandContext(ctx, "swaymsg", fmt.Sprintf("[con_id=%s]", windowID), action)
		} else {
			cmd = exec.CommandContext(ctx, "swaymsg", action)
		}
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to maximize window: " + err.Error()), nil
		}
		return mcp.NewToolResultText("Window maximized"), nil

	case "gnome":
		if windowID != "" {
			cmd := exec.CommandContext(ctx, "wmctrl", "-i", "-r", windowID, "-b", "add,maximized_vert,maximized_horz")
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to maximize window: " + err.Error()), nil
			}
		} else {
			cmd := exec.CommandContext(ctx, "wmctrl", "-r", ":ACTIVE:", "-b", "add,maximized_vert,maximized_horz")
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to maximize window: " + err.Error()), nil
			}
		}
		return mcp.NewToolResultText("Window maximized"), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleTileWindow tiles a window left or right
func (m *MCPServer) handleTileWindow(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	direction, err := request.RequireString("direction")
	if err != nil {
		return mcp.NewToolResultError("direction is required"), nil
	}
	windowID, _ := request.RequireString("window_id")

	if direction != "left" && direction != "right" {
		return mcp.NewToolResultError("direction must be 'left' or 'right'"), nil
	}

	m.logger.Info("tiling window via MCP", "direction", direction, "window_id", windowID)

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		// In Sway tiling WM, we move focus/windows in the layout
		// For a pseudo-tile effect, we can use move commands
		var cmds []*exec.Cmd
		if windowID != "" {
			cmds = append(cmds, exec.CommandContext(ctx, "swaymsg", fmt.Sprintf("[con_id=%s]", windowID), "focus"))
		}
		// Split direction and move
		if direction == "left" {
			cmds = append(cmds, exec.CommandContext(ctx, "swaymsg", "move", "left"))
		} else {
			cmds = append(cmds, exec.CommandContext(ctx, "swaymsg", "move", "right"))
		}
		for _, cmd := range cmds {
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to tile window: " + err.Error()), nil
			}
		}
		return mcp.NewToolResultText(fmt.Sprintf("Window tiled to %s", direction)), nil

	case "gnome":
		// Use xdotool key for GNOME tiling shortcuts (Super+Left/Right)
		key := "super+Left"
		if direction == "right" {
			key = "super+Right"
		}
		if windowID != "" {
			// Focus window first
			cmd := exec.CommandContext(ctx, "wmctrl", "-i", "-a", windowID)
			_ = cmd.Run()
		}
		cmd := exec.CommandContext(ctx, "xdotool", "key", key)
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to tile window: " + err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Window tiled to %s", direction)), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleMoveToWorkspace moves a window to a specific workspace
func (m *MCPServer) handleMoveToWorkspace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workspace, err := request.RequireFloat("workspace")
	if err != nil || workspace < 1 {
		return mcp.NewToolResultError("workspace number is required (1 or greater)"), nil
	}
	windowID, _ := request.RequireString("window_id")

	m.logger.Info("moving window to workspace via MCP", "workspace", int(workspace), "window_id", windowID)

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		var cmd *exec.Cmd
		if windowID != "" {
			cmd = exec.CommandContext(ctx, "swaymsg", fmt.Sprintf("[con_id=%s]", windowID), "move", "container", "to", "workspace", "number", fmt.Sprintf("%d", int(workspace)))
		} else {
			cmd = exec.CommandContext(ctx, "swaymsg", "move", "container", "to", "workspace", "number", fmt.Sprintf("%d", int(workspace)))
		}
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to move window to workspace: " + err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Window moved to workspace %d", int(workspace))), nil

	case "gnome":
		// GNOME uses 0-indexed workspaces internally, but we use 1-indexed for UX
		gnomeWorkspace := int(workspace) - 1
		if windowID != "" {
			cmd := exec.CommandContext(ctx, "wmctrl", "-i", "-r", windowID, "-t", fmt.Sprintf("%d", gnomeWorkspace))
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to move window to workspace: " + err.Error()), nil
			}
		} else {
			cmd := exec.CommandContext(ctx, "wmctrl", "-r", ":ACTIVE:", "-t", fmt.Sprintf("%d", gnomeWorkspace))
			if err := cmd.Run(); err != nil {
				return mcp.NewToolResultError("failed to move window to workspace: " + err.Error()), nil
			}
		}
		return mcp.NewToolResultText(fmt.Sprintf("Window moved to workspace %d", int(workspace))), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleSwitchToWorkspace switches to a specific workspace
func (m *MCPServer) handleSwitchToWorkspace(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	workspace, err := request.RequireFloat("workspace")
	if err != nil || workspace < 1 {
		return mcp.NewToolResultError("workspace number is required (1 or greater)"), nil
	}

	m.logger.Info("switching to workspace via MCP", "workspace", int(workspace))

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		cmd := exec.CommandContext(ctx, "swaymsg", "workspace", "number", fmt.Sprintf("%d", int(workspace)))
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to switch workspace: " + err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Switched to workspace %d", int(workspace))), nil

	case "gnome":
		// GNOME uses 0-indexed workspaces internally
		gnomeWorkspace := int(workspace) - 1
		cmd := exec.CommandContext(ctx, "wmctrl", "-s", fmt.Sprintf("%d", gnomeWorkspace))
		if err := cmd.Run(); err != nil {
			return mcp.NewToolResultError("failed to switch workspace: " + err.Error()), nil
		}
		return mcp.NewToolResultText(fmt.Sprintf("Switched to workspace %d", int(workspace))), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// handleGetWorkspaces lists all workspaces
func (m *MCPServer) handleGetWorkspaces(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	m.logger.Info("getting workspaces via MCP")

	de := m.detectDesktopEnvironment()

	switch de {
	case "sway":
		cmd := exec.CommandContext(ctx, "swaymsg", "-t", "get_workspaces")
		output, err := cmd.Output()
		if err != nil {
			return mcp.NewToolResultError("failed to get workspaces: " + err.Error()), nil
		}
		// Parse and format the output
		formatted := formatSwayWorkspaces(output)
		return mcp.NewToolResultText(formatted), nil

	case "gnome":
		cmd := exec.CommandContext(ctx, "wmctrl", "-d")
		output, err := cmd.Output()
		if err != nil {
			return mcp.NewToolResultError("failed to get workspaces: " + err.Error()), nil
		}
		return mcp.NewToolResultText(string(output)), nil

	default:
		return mcp.NewToolResultError("unsupported desktop environment"), nil
	}
}

// =========================================================================
// Sway JSON Parsing Helpers
// =========================================================================

// swayNode represents a node in the Sway tree (workspace, container, or window)
type swayNode struct {
	ID      int        `json:"id"`
	Name    string     `json:"name"`
	Type    string     `json:"type"`
	Focused bool       `json:"focused"`
	Urgent  bool       `json:"urgent"`
	Output  string     `json:"output"`
	Nodes   []swayNode `json:"nodes"`
	// For floating containers
	FloatingNodes []swayNode `json:"floating_nodes"`
}

// swayWorkspace represents a workspace from swaymsg -t get_workspaces
type swayWorkspace struct {
	Num     int    `json:"num"`
	Name    string `json:"name"`
	Visible bool   `json:"visible"`
	Focused bool   `json:"focused"`
	Urgent  bool   `json:"urgent"`
	Output  string `json:"output"`
}

// formatSwayWindowList parses swaymsg -t get_tree output and returns a formatted window list
func formatSwayWindowList(data []byte) string {
	var root swayNode
	if err := json.Unmarshal(data, &root); err != nil {
		return fmt.Sprintf("Error parsing window tree: %v", err)
	}

	var windows []string
	var collectWindows func(node *swayNode, workspace string)
	collectWindows = func(node *swayNode, workspace string) {
		// Track current workspace
		if node.Type == "workspace" {
			workspace = node.Name
		}

		// If this is a window (con with a name that's not empty)
		if node.Type == "con" && node.Name != "" {
			focusMarker := ""
			if node.Focused {
				focusMarker = " [FOCUSED]"
			}
			windows = append(windows, fmt.Sprintf("  ID: %d | Workspace: %s | Title: %s%s",
				node.ID, workspace, node.Name, focusMarker))
		}

		// Recurse into children
		for i := range node.Nodes {
			collectWindows(&node.Nodes[i], workspace)
		}
		for i := range node.FloatingNodes {
			collectWindows(&node.FloatingNodes[i], workspace)
		}
	}

	collectWindows(&root, "")

	if len(windows) == 0 {
		return "No windows found"
	}

	return fmt.Sprintf("Windows (%d total):\n%s", len(windows), strings.Join(windows, "\n"))
}

// formatSwayWorkspaces parses swaymsg -t get_workspaces output and returns a formatted list
func formatSwayWorkspaces(data []byte) string {
	var workspaces []swayWorkspace
	if err := json.Unmarshal(data, &workspaces); err != nil {
		return fmt.Sprintf("Error parsing workspaces: %v", err)
	}

	if len(workspaces) == 0 {
		return "No workspaces found"
	}

	var lines []string
	for _, ws := range workspaces {
		focusMarker := ""
		if ws.Focused {
			focusMarker = " [FOCUSED]"
		}
		visibleMarker := ""
		if ws.Visible {
			visibleMarker = " (visible)"
		}
		lines = append(lines, fmt.Sprintf("  %d: %s%s%s (output: %s)",
			ws.Num, ws.Name, focusMarker, visibleMarker, ws.Output))
	}

	return fmt.Sprintf("Workspaces (%d total):\n%s", len(workspaces), strings.Join(lines, "\n"))
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
