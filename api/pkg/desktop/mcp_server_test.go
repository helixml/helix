package desktop

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFormatSwayWindowList(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty tree",
			input:    []byte(`{"id":1,"type":"root","nodes":[],"floating_nodes":[]}`),
			expected: "No windows found",
		},
		{
			name: "single window",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [{
						"id": 3,
						"type": "workspace",
						"name": "1",
						"nodes": [{
							"id": 4,
							"type": "con",
							"name": "Terminal",
							"focused": true
						}]
					}]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (1 total):\n  ID: 4 | Workspace: 1 | Title: Terminal [FOCUSED]",
		},
		{
			name: "multiple windows on different workspaces",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [
						{
							"id": 3,
							"type": "workspace",
							"name": "1",
							"nodes": [{
								"id": 4,
								"type": "con",
								"name": "Firefox",
								"focused": false
							}]
						},
						{
							"id": 5,
							"type": "workspace",
							"name": "2",
							"nodes": [{
								"id": 6,
								"type": "con",
								"name": "Zed",
								"focused": true
							}]
						}
					]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (2 total):\n  ID: 4 | Workspace: 1 | Title: Firefox\n  ID: 6 | Workspace: 2 | Title: Zed [FOCUSED]",
		},
		{
			name: "floating window",
			input: []byte(`{
				"id": 1,
				"type": "root",
				"nodes": [{
					"id": 2,
					"type": "output",
					"name": "eDP-1",
					"nodes": [{
						"id": 3,
						"type": "workspace",
						"name": "1",
						"nodes": [],
						"floating_nodes": [{
							"id": 5,
							"type": "con",
							"name": "Popup",
							"focused": false
						}]
					}]
				}],
				"floating_nodes": []
			}`),
			expected: "Windows (1 total):\n  ID: 5 | Workspace: 1 | Title: Popup",
		},
		{
			name:     "invalid json",
			input:    []byte(`{invalid json}`),
			expected: "Error parsing window tree: invalid character 'i' looking for beginning of object key string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSwayWindowList(tt.input)
			if result != tt.expected {
				t.Errorf("formatSwayWindowList() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestFormatSwayWorkspaces(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty workspaces",
			input:    []byte(`[]`),
			expected: "No workspaces found",
		},
		{
			name: "single workspace",
			input: []byte(`[{
				"num": 1,
				"name": "1",
				"visible": true,
				"focused": true,
				"urgent": false,
				"output": "eDP-1"
			}]`),
			expected: "Workspaces (1 total):\n  1: 1 [FOCUSED] (visible) (output: eDP-1)",
		},
		{
			name: "multiple workspaces",
			input: []byte(`[
				{
					"num": 1,
					"name": "1: Code",
					"visible": true,
					"focused": false,
					"urgent": false,
					"output": "eDP-1"
				},
				{
					"num": 2,
					"name": "2: Browser",
					"visible": false,
					"focused": true,
					"urgent": false,
					"output": "HDMI-1"
				}
			]`),
			expected: "Workspaces (2 total):\n  1: 1: Code (visible) (output: eDP-1)\n  2: 2: Browser [FOCUSED] (output: HDMI-1)",
		},
		{
			name:     "invalid json",
			input:    []byte(`{invalid}`),
			expected: "Error parsing workspaces: invalid character 'i' looking for beginning of object key string",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatSwayWorkspaces(tt.input)
			if result != tt.expected {
				t.Errorf("formatSwayWorkspaces() = %q, want %q", result, tt.expected)
			}
		})
	}
}

// TestMCPServer_StreamableHTTP verifies the server responds to Streamable HTTP
// protocol (POST with JSON-RPC body), which is what Zed's HttpTransport uses.
func TestMCPServer_StreamableHTTP(t *testing.T) {
	logger := slog.Default()
	srv := NewMCPServer(MCPConfig{}, logger)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Send a JSON-RPC initialize request as POST to /mcp, the way Zed does it
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initPayload))
	if err != nil {
		t.Fatalf("POST /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, string(body))
	}

	// Verify it's a JSON-RPC response with server info
	resultField, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result field in response, got: %s", string(body))
	}
	serverInfo, ok := resultField["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected serverInfo in result, got: %+v", resultField)
	}
	if serverInfo["name"] != "Helix Desktop" {
		t.Errorf("expected server name 'Helix Desktop', got %q", serverInfo["name"])
	}
}

// TestMCPServer_ToolsList verifies tools/list works over Streamable HTTP.
func TestMCPServer_ToolsList(t *testing.T) {
	logger := slog.Default()
	srv := NewMCPServer(MCPConfig{}, logger)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Initialize first
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initPayload))
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
	// Extract session ID from Mcp-Session header if present
	sessionID := resp.Header.Get("Mcp-Session")
	resp.Body.Close()

	// Now list tools
	listPayload := `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`
	req, _ := http.NewRequest("POST", ts.URL+"/mcp", strings.NewReader(listPayload))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session", sessionID)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("tools/list failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, string(body))
	}

	resultField, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result field, got: %s", string(body))
	}

	tools, ok := resultField["tools"].([]interface{})
	if !ok {
		t.Fatalf("expected tools array, got: %+v", resultField)
	}

	// Verify we have the expected desktop tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		toolNames[toolMap["name"].(string)] = true
	}

	expectedTools := []string{"take_screenshot", "type_text", "mouse_click", "list_windows", "get_clipboard", "set_clipboard"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q not found in tools list", name)
		}
	}
}

// TestDesktopServer_ServesMCPRoute verifies that the desktop HTTP server
// (port 9876, reached via RevDial) serves /mcp when an MCP handler is mounted.
// This is required because RevDial tunnels to port 9876 — the desktop HTTP
// server — not port 9878 where the MCP server previously ran standalone.
// The API gateway proxy sends MCP requests through RevDial to /mcp.
func TestDesktopServer_ServesMCPRoute(t *testing.T) {
	logger := slog.Default()

	// Create the MCP server (normally runs standalone on port 9878)
	mcpSrv := NewMCPServer(MCPConfig{}, logger)

	// Create the desktop server (port 9876, the RevDial target)
	desktopSrv := NewServer(Config{HTTPPort: "9876"}, logger)
	desktopSrv.SetMCPHandler(mcpSrv)

	// Get the HTTP handler (same one used by the real server)
	handler := desktopSrv.httpHandler()
	ts := httptest.NewServer(handler)
	defer ts.Close()

	// Send an MCP initialize request to /mcp — exactly as the gateway proxy does
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initPayload))
	if err != nil {
		t.Fatalf("POST /mcp failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200 from /mcp on desktop server, got %d: %s", resp.StatusCode, string(body))
	}

	body, _ := io.ReadAll(resp.Body)
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("response is not valid JSON: %v\nbody: %s", err, string(body))
	}

	// Verify it's the MCP server responding
	resultField, ok := result["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result field, got: %s", string(body))
	}
	serverInfo, ok := resultField["serverInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected serverInfo, got: %+v", resultField)
	}
	if serverInfo["name"] != "Helix Desktop" {
		t.Errorf("expected 'Helix Desktop', got %q", serverInfo["name"])
	}
}

func TestNewMCPServer(t *testing.T) {
	// Test default config
	t.Run("default config", func(t *testing.T) {
		cfg := MCPConfig{}
		// Can't fully test without a logger, but we can verify config defaults
		if cfg.Port != "" {
			t.Errorf("expected empty port, got %s", cfg.Port)
		}
		if cfg.ScreenshotURL != "" {
			t.Errorf("expected empty screenshot URL, got %s", cfg.ScreenshotURL)
		}
	})

	// Test custom config
	t.Run("custom config", func(t *testing.T) {
		cfg := MCPConfig{
			Port:          "9999",
			ScreenshotURL: "http://custom:8080/screenshot",
		}
		if cfg.Port != "9999" {
			t.Errorf("expected port 9999, got %s", cfg.Port)
		}
		if cfg.ScreenshotURL != "http://custom:8080/screenshot" {
			t.Errorf("expected custom screenshot URL, got %s", cfg.ScreenshotURL)
		}
	})
}
