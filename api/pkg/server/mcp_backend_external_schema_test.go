package server

// Regression test for helix-specs task 002204_the-one-blocker.
//
// It exercises the REAL production helper buildProxyTool (extracted from
// ExternalMCPBackend.getOrCreateServer) — the code path a zed_external org Bot
// actually uses when it loads create_bot/subscribe over the external MCP proxy.
//
// The org MCP server publishes create_bot.tools as an array (proven by
// TestSchemaWireArrayParams). This test asserts the proxy preserves that array
// when re-advertising to Zed. Against the current WithString-only
// reconstruction it FAILS (tools comes back type:"string") — the exact
// "cannot unmarshal string into []string" blocker.

import (
	"encoding/json"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// upstreamCreateBotTool mimics what externalClient.ListTools() returns from the
// org MCP server for create_bot: `tools` is a proper array-of-string.
func upstreamCreateBotTool() mcp.Tool {
	return mcp.Tool{
		Name:        "create_bot",
		Description: "Create a new Bot in one call.",
		InputSchema: mcp.ToolInputSchema{
			Type: "object",
			Properties: map[string]any{
				"content": map[string]any{
					"type":        "string",
					"description": "The bot's prompt.",
				},
				"tools": map[string]any{
					"type":        "array",
					"description": "MCP tools to grant.",
					"items":       map[string]any{"type": "string"},
				},
			},
			Required: []string{"content", "tools"},
		},
	}
}

// servedParamType marshals a proxied tool exactly as it goes over the wire to
// Zed and returns properties.<param>.type as the client would parse it.
func servedParamType(t *testing.T, tool mcp.Tool, param string) string {
	t.Helper()
	raw, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("marshal served tool: %v", err)
	}
	var parsed struct {
		InputSchema struct {
			Properties map[string]struct {
				Type json.RawMessage `json:"type"`
				Items *struct {
					Type string `json:"type"`
				} `json:"items"`
			} `json:"properties"`
		} `json:"inputSchema"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal served tool: %v", err)
	}
	var typ string
	_ = json.Unmarshal(parsed.InputSchema.Properties[param].Type, &typ)
	return typ
}

// TestBuildProxyTool_PreservesArrayParams pins the blocker on the real
// production helper. RED until buildProxyTool stops flattening arrays.
func TestBuildProxyTool_PreservesArrayParams(t *testing.T) {
	served := buildProxyTool(upstreamCreateBotTool())

	if got := servedParamType(t, served, "tools"); got != "array" {
		t.Fatalf("create_bot.tools re-advertised as %q, want \"array\" — the proxy flattened the array to string (cannot unmarshal string into []string)", got)
	}
	// Scalars must stay intact once the array is fixed.
	if got := servedParamType(t, served, "content"); got != "string" {
		t.Fatalf("create_bot.content = %q, want \"string\"", got)
	}
}
