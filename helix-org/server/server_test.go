package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}

	reg := tools.NewRegistry()
	if err := reg.Register(tools.Ping{}); err != nil {
		t.Fatalf("register ping: %v", err)
	}

	srv := httptest.NewServer(server.New(s, reg, nil, nil, "").Handler())
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
	rootID := root.ID
	child, _ := domain.NewPosition("p-engineering", "r-ceo", &rootID)
	if err := s.Positions.Create(ctx, child); err != nil {
		t.Fatalf("seed child: %v", err)
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
	return srv
}

type envelope struct {
	Data   json.RawMessage `json:"data"`
	Errors json.RawMessage `json:"errors"`
}

// withGet performs a GET against url and invokes fn with the response. The
// response body is closed after fn returns; this shape keeps bodyclose happy
// because Do and Close sit in the same function.
func withGet(t *testing.T, url string, fn func(res *http.Response, env envelope)) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer func() { _ = res.Body.Close() }()
	var env envelope
	if err := json.NewDecoder(res.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	fn(res, env)
}

func TestGetRole(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	withGet(t, srv.URL+"/roles/r-ceo", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", res.StatusCode)
		}
		if got := res.Header.Get("Content-Type"); got != server.MediaType {
			t.Fatalf("content-type = %q, want %q", got, server.MediaType)
		}
		var resource server.Resource
		if err := json.Unmarshal(env.Data, &resource); err != nil {
			t.Fatalf("unmarshal data: %v", err)
		}
		if resource.Type != "roles" || resource.ID != "r-ceo" {
			t.Fatalf("resource = %+v", resource)
		}
	})
}

func TestGetRoleNotFound(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	withGet(t, srv.URL+"/roles/missing", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", res.StatusCode)
		}
		if len(env.Errors) == 0 {
			t.Fatalf("expected errors, got data: %s", env.Data)
		}
	})
}

func TestListPositions(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	withGet(t, srv.URL+"/positions", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var resources []server.Resource
		if err := json.Unmarshal(env.Data, &resources); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resources) != 2 {
			t.Fatalf("positions = %d, want 2", len(resources))
		}
	})
}

func TestListPositionChildren(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)
	withGet(t, srv.URL+"/positions/p-root/children", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status = %d", res.StatusCode)
		}
		var resources []server.Resource
		if err := json.Unmarshal(env.Data, &resources); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resources) != 1 || resources[0].ID != "p-engineering" {
			t.Fatalf("children = %+v", resources)
		}
	})
}

func TestWorkerAndGrants(t *testing.T) {
	t.Parallel()
	srv := newTestServer(t)

	withGet(t, srv.URL+"/workers/w-ceo", func(res *http.Response, _ envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("worker status = %d", res.StatusCode)
		}
	})

	withGet(t, srv.URL+"/workers/w-ceo/grants", func(res *http.Response, env envelope) {
		if res.StatusCode != http.StatusOK {
			t.Fatalf("grants status = %d", res.StatusCode)
		}
		var resources []server.Resource
		if err := json.Unmarshal(env.Data, &resources); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(resources) != 2 {
			t.Fatalf("grants = %+v", resources)
		}
	})
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
	srv := newTestServer(t)
	session := connectMCP(t, srv.URL, "w-ceo")

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
	srv := newTestServer(t)
	session := connectMCP(t, srv.URL, "w-ceo")

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
	srv := newTestServer(t)
	session := connectMCP(t, srv.URL, "w-ceo")

	_, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "create_role",
		Arguments: map[string]any{"id": "r-x", "title": "X"},
	})
	if err == nil {
		t.Fatalf("expected error for ungranted tool, got nil")
	}
}
