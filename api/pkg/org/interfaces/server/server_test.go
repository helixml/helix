package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/prompts"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
)

// newTestServer seeds a CEO Bot whose Tools list both ping and
// create_bot — but only ping is actually registered with the server.
// That lets us assert the MCP surface is the intersection of (a)
// Bot.Tools and (b) tools the server knows. Returns the running
// httptest.Server and the botID to act as.
func newTestServer(t *testing.T) (*httptest.Server, orgchart.BotID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	if err := reg.Register(mcptools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	bot, _ := orgchart.NewBot(
		"b-ceo",
		"# CEO\nTop of org.",
		[]tool.Name{mcptools.PingName, "create_bot"},
		nil,
		time.Now().UTC(),
		"org-test",
	)
	if err := s.Bots.Create(ctx, bot); err != nil {
		t.Fatalf("seed bot: %v", err)
	}
	return srv, "b-ceo"
}

// connectMCP returns an MCP client session bound to the given bot's /mcp
// endpoint. The URL keeps the `/workers/` path segment for compatibility
// with the helix MCP backend rewrite; {id} is a Bot ID. The session is
// closed when the test ends.
func connectMCP(t *testing.T, baseURL string, botID orgchart.BotID) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "helix-org-test", Version: "v0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             baseURL + "/orgs/org-test/workers/" + string(botID) + "/mcp",
		DisableStandaloneSSE: true,
	}
	session, err := c.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("mcp connect %s: %v", botID, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// newTestServerToolDerived seeds a Bot whose MCP surface comes from
// Bot.Tools. The Bot lists ping. Asserting that ping appears on the
// Bot's MCP endpoint pins the "Bot.Tools is the live source of truth"
// contract.
func newTestServerToolDerived(t *testing.T) (*httptest.Server, orgchart.BotID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	if err := reg.Register(mcptools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	bot, _ := orgchart.NewBot(
		"b-ceo",
		"# CEO\nTop of org.",
		[]tool.Name{mcptools.PingName},
		nil,
		time.Now().UTC(),
		"org-test",
	)
	if err := s.Bots.Create(ctx, bot); err != nil {
		t.Fatalf("seed bot: %v", err)
	}
	return srv, "b-ceo"
}

// TestMCPListToolsFromBot pins the contract: a Bot's MCP surface is
// derived live from their Bot.Tools. Creating a Bot with `ping` listed
// must make ping appear on the MCP endpoint without any explicit
// tool-assignment call.
func TestMCPListToolsFromBot(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServerToolDerived(t)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	if !got["ping"] {
		t.Errorf("ping missing from bot-derived list: %+v", got)
	}
}

// TestMCPListTools confirms that the MCP tool list a bot sees is the
// intersection of (a) their Bot.Tools and (b) tools the server has
// actually registered. The CEO's Bot lists both ping and create_bot, but
// only ping is registered on the test registry — so create_bot must not
// appear. update_bot is in neither the bot nor the registry.
func TestMCPListTools(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServer(t)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tool := range res.Tools {
		got[tool.Name] = true
	}
	if !got["ping"] {
		t.Errorf("ping missing from list: %+v", got)
	}
	if got["create_bot"] {
		t.Errorf("bot-listed-but-unregistered tool create_bot leaked into list")
	}
	if got["update_bot"] {
		t.Errorf("bot-omitted tool update_bot appeared in list")
	}
}

// TestMCPInvokePing exercises a tool over MCP end-to-end: the CEO's Bot
// lists ping, so calling tools/call should succeed and echo the message
// back along with the caller ID.
func TestMCPInvokePing(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServer(t)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "ping",
		Arguments: map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("tool reported error: %+v", res.Content)
	}
	if len(res.Content) == 0 {
		t.Fatalf("empty content: %+v", res)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("content[0] = %T, want *TextContent", res.Content[0])
	}
	var payload struct {
		Echo   string `json:"echo"`
		Caller string `json:"caller"`
	}
	if err := json.Unmarshal([]byte(text.Text), &payload); err != nil {
		t.Fatalf("decode tool result: %v", err)
	}
	if payload.Echo != "hello" || payload.Caller != "b-ceo" {
		t.Fatalf("payload = %+v", payload)
	}
}

// TestMCPToolNotInBotHidden confirms that a tool not listed in the Bot's
// Tools isn't visible. Calling a hidden tool surfaces as a
// protocol-level "tool not found", not a 403 — the LLM never sees tools
// its Bot doesn't carry.
func TestMCPToolNotInBotHidden(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServer(t)
	session := connectMCP(t, srv.URL, botID)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "update_bot",
		Arguments: map[string]any{"id": "b-x", "content": "X"},
	})
	if err == nil {
		t.Fatalf("expected error for tool not in Bot, got nil")
	}
}

// newTestServerWithPrompts mirrors newTestServer but also attaches a
// prompts registry containing the role prompt. Whether the bot actually
// sees the prompt depends on whether their Bot.Tools includes the gating
// tool (create_bot); callers exercise both branches.
func newTestServerWithPrompts(t *testing.T, includeCreateBot bool) (*httptest.Server, orgchart.BotID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	if err := reg.Register(mcptools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}
	if err := mcptools.RegisterBuiltins(reg, mcptools.DefaultDeps(s).Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}

	promptReg := prompts.NewRegistry()
	if err := promptReg.Register(prompts.Role{}); err != nil {
		t.Fatalf("register role prompt: %v", err)
	}

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).WithPrompts(promptReg).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	botTools := []tool.Name{mcptools.PingName}
	if includeCreateBot {
		botTools = append(botTools, mcptools.CreateBotName)
	}
	bot, _ := orgchart.NewBot("b-ceo", "# CEO", botTools, nil, time.Now().UTC(), "org-test")
	_ = s.Bots.Create(ctx, bot)
	return srv, "b-ceo"
}

// TestMCPListPromptsVisibleWhenBotHasTool confirms that a prompt gated on
// a tool shows up exactly when the bot's Tools includes it.
func TestMCPListPromptsVisibleWhenBotHasTool(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServerWithPrompts(t, true)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	got := make(map[string]bool, len(res.Prompts))
	for _, p := range res.Prompts {
		got[p.Name] = true
	}
	if !got[string(prompts.RoleName)] {
		t.Errorf("role prompt missing from list: %+v", got)
	}
}

// TestMCPListPromptsHiddenWhenBotLacksTool confirms the gating: a bot
// whose Tools don't list create_bot does NOT see the role prompt,
// because the final tool call would fail anyway.
func TestMCPListPromptsHiddenWhenBotLacksTool(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServerWithPrompts(t, false)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	for _, p := range res.Prompts {
		if p.Name == string(prompts.RoleName) {
			t.Errorf("role prompt visible without create_bot tool: %+v", p)
		}
	}
}

// TestMCPGetPromptReturnsSeedMessages exercises the full prompts/get
// round-trip: the rendered template lands as the user-role seed message
// in the conversation.
func TestMCPGetPromptReturnsSeedMessages(t *testing.T) {
	t.Parallel()
	srv, botID := newTestServerWithPrompts(t, true)
	session := connectMCP(t, srv.URL, botID)

	res, err := session.GetPrompt(context.Background(), &mcp.GetPromptParams{
		Name:      string(prompts.RoleName),
		Arguments: map[string]string{"hint": "VP marketing"},
	})
	if err != nil {
		t.Fatalf("get prompt: %v", err)
	}
	if len(res.Messages) != 1 {
		t.Fatalf("messages = %d, want 1", len(res.Messages))
	}
	msg := res.Messages[0]
	if msg.Role != "user" {
		t.Errorf("role = %q, want user", msg.Role)
	}
	text, ok := msg.Content.(*mcp.TextContent)
	if !ok {
		t.Fatalf("content = %T, want *TextContent", msg.Content)
	}
	if !strings.Contains(text.Text, "VP marketing") {
		t.Errorf("hint not threaded through: %s", text.Text)
	}
	if !strings.Contains(text.Text, "create_bot") {
		t.Errorf("template missing create_bot reference")
	}
}
