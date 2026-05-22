package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/domain"
	"github.com/helixml/helix/api/pkg/org/prompts"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/store/sqlite"
	"github.com/helixml/helix/api/pkg/org/tools"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/api/pkg/org/server"
)

// newTestServer seeds a CEO Worker with a ping grant and a hire_worker
// grant (the latter pointing at a tool deliberately not registered, so
// we can assert it's filtered out of the MCP list). Returns the running
// httptest.Server and the workerID to act as.
func newTestServer(t *testing.T) (*httptest.Server, worker.ID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	reg := tools.NewRegistry()
	if err := reg.Register(tools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	role, _ := role.New("r-ceo", "# CEO\nTop of org.", nil, nil, time.Now().UTC())
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	root, _ := domain.NewPosition("p-root", "r-ceo", nil)
	if err := s.Positions.Create(ctx, root); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	ai, _ := domain.NewAIWorker("w-ceo", "p-root", "")
	if err := s.Workers.Create(ctx, ai); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	grant, _ := domain.NewToolGrant("g-1", "w-ceo", "hire_worker")
	if err := s.Grants.Create(ctx, grant); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	pingGrant, _ := domain.NewToolGrant("g-ping", "w-ceo", tools.PingName)
	if err := s.Grants.Create(ctx, pingGrant); err != nil {
		t.Fatalf("seed ping grant: %v", err)
	}
	return srv, "w-ceo"
}

// connectMCP returns an MCP client session bound to the given worker's
// /mcp endpoint. The session is closed when the test ends.
func connectMCP(t *testing.T, baseURL string, workerID worker.ID) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "helix-org-test", Version: "v0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             baseURL + "/workers/" + string(workerID) + "/mcp",
		DisableStandaloneSSE: true,
	}
	session, err := c.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("mcp connect %s: %v", workerID, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// TestMCPListTools confirms that the MCP tool list a worker sees is the
// intersection of (a) their grants and (b) tools the server has actually
// registered. The CEO holds grants for both ping and hire_worker, but
// only ping is registered on the test registry — so hire_worker must
// not appear. create_role is neither granted nor registered.
func TestMCPListTools(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServer(t)
	session := connectMCP(t, srv.URL, workerID)

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
	if got["hire_worker"] {
		t.Errorf("granted-but-unregistered tool hire_worker leaked into list")
	}
	if got["create_role"] {
		t.Errorf("ungranted tool create_role appeared in list")
	}
}

// TestMCPInvokePing exercises a granted tool over MCP end-to-end: the
// CEO holds a ping grant, so calling tools/call should succeed and echo
// the message back along with the caller ID.
func TestMCPInvokePing(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServer(t)
	session := connectMCP(t, srv.URL, workerID)

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
	if payload.Echo != "hello" || payload.Caller != "w-ceo" {
		t.Fatalf("payload = %+v", payload)
	}
}

// TestMCPUngrantedToolHidden confirms that a tool the worker doesn't
// hold isn't visible. Calling a hidden tool surfaces as a protocol-level
// "tool not found", not a 403 — the LLM never sees ungranted tools at all.
func TestMCPUngrantedToolHidden(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServer(t)
	session := connectMCP(t, srv.URL, workerID)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_role",
		Arguments: map[string]any{"id": "r-x", "title": "X"},
	})
	if err == nil {
		t.Fatalf("expected error for ungranted tool, got nil")
	}
}

// newTestServerWithPrompts mirrors newTestServer but also attaches a
// prompts registry containing new_role. Whether the worker actually
// sees the prompt depends on whether they hold the gating grant
// (create_role); callers exercise both branches.
func newTestServerWithPrompts(t *testing.T, grantCreateRole bool) (*httptest.Server, worker.ID) {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	reg := tools.NewRegistry()
	if err := reg.Register(tools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}
	if err := tools.RegisterBuiltins(reg, tools.DefaultDeps(s)); err != nil {
		t.Fatalf("register builtins: %v", err)
	}

	promptReg := prompts.NewRegistry()
	if err := promptReg.Register(prompts.Role{}); err != nil {
		t.Fatalf("register new_role: %v", err)
	}

	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).WithPrompts(promptReg).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	role, _ := role.New("r-ceo", "# CEO", nil, nil, time.Now().UTC())
	_ = s.Roles.Create(ctx, role)
	root, _ := domain.NewPosition("p-root", "r-ceo", nil)
	_ = s.Positions.Create(ctx, root)
	ai, _ := domain.NewAIWorker("w-ceo", "p-root", "")
	_ = s.Workers.Create(ctx, ai)
	pingGrant, _ := domain.NewToolGrant("g-ping", "w-ceo", tools.PingName)
	_ = s.Grants.Create(ctx, pingGrant)
	if grantCreateRole {
		g, _ := domain.NewToolGrant("g-create-role", "w-ceo", tools.CreateRoleName)
		if err := s.Grants.Create(ctx, g); err != nil {
			t.Fatalf("seed create_role grant: %v", err)
		}
	}
	return srv, "w-ceo"
}

// TestMCPListPromptsVisibleWithGrant confirms that a prompt gated on a
// tool grant shows up exactly when the worker holds that grant.
func TestMCPListPromptsVisibleWithGrant(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServerWithPrompts(t, true)
	session := connectMCP(t, srv.URL, workerID)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	got := make(map[string]bool, len(res.Prompts))
	for _, p := range res.Prompts {
		got[p.Name] = true
	}
	if !got[string(prompts.RoleName)] {
		t.Errorf("new_role missing from list: %+v", got)
	}
}

// TestMCPListPromptsHiddenWithoutGrant confirms the gating: a worker
// without create_role does NOT see the new_role prompt, because the
// final tool call would fail anyway.
func TestMCPListPromptsHiddenWithoutGrant(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServerWithPrompts(t, false)
	session := connectMCP(t, srv.URL, workerID)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	for _, p := range res.Prompts {
		if p.Name == string(prompts.RoleName) {
			t.Errorf("new_role visible without create_role grant: %+v", p)
		}
	}
}

// TestMCPGetPromptReturnsSeedMessages exercises the full prompts/get
// round-trip: the rendered template lands as the user-role seed message
// in the conversation.
func TestMCPGetPromptReturnsSeedMessages(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServerWithPrompts(t, true)
	session := connectMCP(t, srv.URL, workerID)

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
	if !strings.Contains(text.Text, "create_role") {
		t.Errorf("template missing create_role reference")
	}
}
