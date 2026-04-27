package server_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
)

// newTestServer seeds a CEO Worker with a ping grant and a hire_worker
// grant (the latter pointing at a tool deliberately not registered, so
// we can assert it's filtered out of the MCP list). Returns the running
// httptest.Server and the workerID to act as.
func newTestServer(t *testing.T) (*httptest.Server, domain.WorkerID) {
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
	role, _ := domain.NewRole("r-ceo", "# CEO\nTop of org.", time.Now().UTC())
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("seed role: %v", err)
	}
	root, _ := domain.NewPosition("p-root", "r-ceo", nil)
	if err := s.Positions.Create(ctx, root); err != nil {
		t.Fatalf("seed root: %v", err)
	}
	ai, _ := domain.NewAIWorker("w-ceo", []domain.PositionID{"p-root"})
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
func connectMCP(t *testing.T, baseURL string, workerID domain.WorkerID) *mcp.ClientSession {
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
