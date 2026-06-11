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
	"github.com/helixml/helix/api/pkg/org/application/tools"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
)

// newTestServer seeds a CEO Worker whose Role lists both ping and
// hire_worker — but only ping is actually registered with the server.
// That lets us assert the MCP surface is the intersection of (a)
// Role.Tools and (b) tools the server knows. Returns the running
// httptest.Server and the workerID to act as.
func newTestServer(t *testing.T) (*httptest.Server, orgchart.WorkerID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

	reg := tools.NewRegistry()
	if err := reg.Register(tools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	role, _ := orgchart.NewRole(
		"r-ceo",
		"# CEO\nTop of org.",
		[]tool.Name{tools.PingName, "hire_worker"},
		nil,
		time.Now().UTC(),
		"org-test",
	)
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	ai, _ := orgchart.NewAIWorker("w-ceo", "r-ceo", "", "org-test")
	if err := s.Workers.Create(ctx, ai); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	return srv, "w-ceo"
}

// connectMCP returns an MCP client session bound to the given worker's
// /mcp endpoint. The session is closed when the test ends.
func connectMCP(t *testing.T, baseURL string, workerID orgchart.WorkerID) *mcp.ClientSession {
	t.Helper()
	c := mcp.NewClient(&mcp.Implementation{Name: "helix-org-test", Version: "v0.0.0"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:             baseURL + "/orgs/org-test/workers/" + string(workerID) + "/mcp",
		DisableStandaloneSSE: true,
	}
	session, err := c.Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatalf("mcp connect %s: %v", workerID, err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

// newTestServerRoleDerived seeds a Worker whose MCP surface comes from
// Role.Tools rather than a per-Worker tool record. The Role lists ping.
// Asserting that ping appears on the Worker's MCP endpoint pins the
// "Role.Tools is the live source of truth" contract — see
// feat/org-role-tools-as-source-of-truth.
func newTestServerRoleDerived(t *testing.T) (*httptest.Server, orgchart.WorkerID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

	reg := tools.NewRegistry()
	if err := reg.Register(tools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	role, _ := orgchart.NewRole(
		"r-ceo",
		"# CEO\nTop of org.",
		[]tool.Name{tools.PingName},
		nil,
		time.Now().UTC(),
		"org-test",
	)
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	ai, _ := orgchart.NewAIWorker("w-ceo", "r-ceo", "", "org-test")
	if err := s.Workers.Create(ctx, ai); err != nil {
		t.Fatalf("seed worker: %v", err)
	}
	return srv, "w-ceo"
}

// TestMCPListToolsFromRole pins the contract: a Worker's MCP surface
// is derived live from their Role.Tools, with no per-Worker tool record
// involved. Hiring a Worker into a Role with `ping` listed must make
// ping appear on the MCP endpoint without any explicit tool-assignment call.
func TestMCPListToolsFromRole(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServerRoleDerived(t)
	session := connectMCP(t, srv.URL, workerID)

	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	if !got["ping"] {
		t.Errorf("ping missing from role-derived list: %+v", got)
	}
}

// TestMCPListTools confirms that the MCP tool list a worker sees is the
// intersection of (a) their Role.Tools and (b) tools the server has
// actually registered. The CEO's Role lists both ping and hire_worker,
// but only ping is registered on the test registry — so hire_worker
// must not appear. create_role is in neither the role nor the registry.
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
		t.Errorf("role-listed-but-unregistered tool hire_worker leaked into list")
	}
	if got["create_role"] {
		t.Errorf("role-omitted tool create_role appeared in list")
	}
}

// TestMCPInvokePing exercises a tool over MCP end-to-end: the CEO's
// Role lists ping, so calling tools/call should succeed and echo
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

// TestMCPToolNotInRoleHidden confirms that a tool not listed in the
// Worker's Role.Tools isn't visible. Calling a hidden tool surfaces as
// a protocol-level "tool not found", not a 403 — the LLM never sees
// tools its Role doesn't carry.
func TestMCPToolNotInRoleHidden(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServer(t)
	session := connectMCP(t, srv.URL, workerID)

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_role",
		Arguments: map[string]any{"id": "r-x", "title": "X"},
	})
	if err == nil {
		t.Fatalf("expected error for tool not in Role, got nil")
	}
}

// newTestServerWithPrompts mirrors newTestServer but also attaches a
// prompts registry containing new_role. Whether the worker actually
// sees the prompt depends on whether their Role.Tools includes the
// gating tool (create_role); callers exercise both branches.
func newTestServerWithPrompts(t *testing.T, includeCreateRole bool) (*httptest.Server, orgchart.WorkerID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)

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

	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).WithPrompts(promptReg).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	roleTools := []tool.Name{tools.PingName}
	if includeCreateRole {
		roleTools = append(roleTools, tools.CreateRoleName)
	}
	role, _ := orgchart.NewRole("r-ceo", "# CEO", roleTools, nil, time.Now().UTC(), "org-test")
	_ = s.Roles.Create(ctx, role)
	ai, _ := orgchart.NewAIWorker("w-ceo", "r-ceo", "", "org-test")
	_ = s.Workers.Create(ctx, ai)
	return srv, "w-ceo"
}

// TestMCPListPromptsVisibleWhenRoleHasTool confirms that a prompt gated
// on a tool shows up exactly when the worker's Role.Tools includes it.
func TestMCPListPromptsVisibleWhenRoleHasTool(t *testing.T) {
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

// TestMCPListPromptsHiddenWhenRoleLacksTool confirms the gating: a worker
// whose Role doesn't list create_role does NOT see the new_role
// prompt, because the final tool call would fail anyway.
func TestMCPListPromptsHiddenWhenRoleLacksTool(t *testing.T) {
	t.Parallel()
	srv, workerID := newTestServerWithPrompts(t, false)
	session := connectMCP(t, srv.URL, workerID)

	res, err := session.ListPrompts(context.Background(), nil)
	if err != nil {
		t.Fatalf("list prompts: %v", err)
	}
	for _, p := range res.Prompts {
		if p.Name == string(prompts.RoleName) {
			t.Errorf("new_role visible without create_role tool: %+v", p)
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
