package session

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMCPServer_StreamableHTTP verifies the server responds to Streamable HTTP
// protocol (POST with JSON-RPC body), which is what Zed's HttpTransport uses.
func TestMCPServer_StreamableHTTP(t *testing.T) {
	logger := slog.Default()
	srv := NewMCPServer(MCPConfig{
		HelixAPIURL:   "http://localhost:8080",
		HelixAPIToken: "test-token",
		SessionID:     "test-session",
	}, logger)

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
	if serverInfo["name"] != "Helix Session" {
		t.Errorf("expected server name 'Helix Session', got %q", serverInfo["name"])
	}
}

// TestMCPServer_ToolsList verifies tools/list works over Streamable HTTP.
func TestMCPServer_ToolsList(t *testing.T) {
	logger := slog.Default()
	srv := NewMCPServer(MCPConfig{
		HelixAPIURL:   "http://localhost:8080",
		HelixAPIToken: "test-token",
		SessionID:     "test-session",
	}, logger)

	ts := httptest.NewServer(srv)
	defer ts.Close()

	// Initialize first
	initPayload := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}`
	resp, err := http.Post(ts.URL+"/mcp", "application/json", strings.NewReader(initPayload))
	if err != nil {
		t.Fatalf("initialize failed: %v", err)
	}
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

	// Verify we have the expected session tools
	toolNames := make(map[string]bool)
	for _, tool := range tools {
		toolMap := tool.(map[string]interface{})
		toolNames[toolMap["name"].(string)] = true
	}

	expectedTools := []string{"current_session", "session_toc", "get_turn", "search_session", "list_sessions"}
	for _, name := range expectedTools {
		if !toolNames[name] {
			t.Errorf("expected tool %q not found in tools list", name)
		}
	}
}
