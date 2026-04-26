package tools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/server"
	"github.com/helixml/helix-org/store/sqlite"
	"github.com/helixml/helix-org/tools"
)

// TestDemoOwnerHiresCEO walks the "manager does the orchestration" story
// over MCP: each tool does one primitive thing, and the test drives the
// hiring ritual step by step.
//
// Owner is pre-seeded. Owner creates a #general Stream, subscribes
// themselves, defines a CEO Role (markdown content), creates a Position,
// then hires the CEO with inline grants and an identityContent. The
// Worker's role.md / identity.md / agent.md are written under EnvsDir
// by hire_worker. Owner publishes; CEO sees it.
func TestDemoOwnerHiresCEO(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	envsDir := t.TempDir()

	reg := tools.NewRegistry()
	deps := tools.DefaultDeps(s)
	deps.EnvsDir = envsDir
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.New(s, reg, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// Seed owner directly: role, position, worker, environment, structural grants.
	now := time.Now().UTC()
	ownerRole, err := domain.NewRole("r-owner", "# Owner\nBootstrap owner.", now)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"})
	mustCreate(t, s.Workers.Create(ctx, owner))
	ownerEnvPath := filepath.Join(envsDir, "w-owner")
	if err := os.MkdirAll(ownerEnvPath, 0o750); err != nil {
		t.Fatalf("mkdir owner env: %v", err)
	}
	ownerEnv, _ := domain.NewEnvironment("w-owner", ownerEnvPath, now)
	mustCreate(t, s.Environments.Create(ctx, ownerEnv))
	for _, name := range []domain.ToolName{
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.CreatePositionName,
		tools.HireWorkerName,
		tools.GrantToolName,
		tools.CreateStreamName,
		tools.SubscribeName,
		tools.PublishName,
	} {
		grantID := domain.GrantID("g-owner-" + name)
		g, _ := domain.NewToolGrant(grantID, "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, tools.CreateStreamName, map[string]any{
		"id":   "s-general",
		"name": "general",
	})
	invokeOK(t, ownerSession, tools.SubscribeName, map[string]any{"streamId": "s-general"})

	invokeExpectID(t, ownerSession, tools.CreateRoleName, map[string]any{
		"id":      "r-ceo",
		"content": "# CEO\nLead the company. Subscribe to s-general.",
	})

	invokeExpectID(t, ownerSession, tools.CreatePositionName, map[string]any{
		"id":       "p-ceo",
		"roleId":   "r-ceo",
		"parentId": "p-root",
	})

	invokeExpectID(t, ownerSession, tools.HireWorkerName, map[string]any{
		"id":              "w-ceo",
		"positionId":      "p-ceo",
		"kind":            "ai",
		"identityContent": "# Meina Gladstone\nCEO. Decisive, warm, direct.",
		"grants": []map[string]any{
			{"toolName": "publish"},
			{"toolName": "subscribe"},
		},
	})

	// hire_worker stamps the trio of files under EnvsDir/<id>/.
	ceoEnvPath := filepath.Join(envsDir, "w-ceo")
	for _, name := range []string{"role.md", "identity.md", "agent.md"} {
		data, err := os.ReadFile(filepath.Join(ceoEnvPath, name)) //nolint:gosec // path is t.TempDir() + known filename
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s is empty", name)
		}
	}

	// Stand in for the CEO's hire activation: subscribe to the
	// stream they were told about. The dispatcher isn't wired in
	// this test, so we drive it manually.
	ceoSession := connectMCP(t, srv.URL, "w-ceo")
	invokeOK(t, ceoSession, tools.SubscribeName, map[string]any{"streamId": "s-general"})

	if _, err := s.Subscriptions.Find(ctx, "w-ceo", "s-general"); err != nil {
		t.Fatalf("CEO subscription on s-general missing: %v", err)
	}

	invokeExpectID(t, ownerSession, tools.PublishName, map[string]any{
		"streamId": "s-general",
		"body":     "please hire all of your staff",
	})
	ceoEvents, err := s.Events.ListForWorker(ctx, "w-ceo", 10)
	if err != nil {
		t.Fatalf("ceo events: %v", err)
	}
	if len(ceoEvents) != 1 || ceoEvents[0].Body != "please hire all of your staff" {
		t.Fatalf("ceo events = %+v", ceoEvents)
	}
}

// TestUpdateRoleFanOut hires two workers under the same Role, runs
// update_role, and asserts both Workers' role.md is rewritten.
func TestUpdateRoleFanOut(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	envsDir := t.TempDir()

	reg := tools.NewRegistry()
	deps := tools.DefaultDeps(s)
	deps.EnvsDir = envsDir
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.New(s, reg, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := domain.NewRole("r-owner", "# Owner", now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"})
	mustCreate(t, s.Workers.Create(ctx, owner))
	for _, name := range []domain.ToolName{
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.CreatePositionName,
		tools.HireWorkerName,
	} {
		g, _ := domain.NewToolGrant(domain.GrantID("g-"+name), "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, tools.CreateRoleName, map[string]any{
		"id":      "r-eng",
		"content": "# Engineer v1\nBuild stuff.",
	})
	invokeExpectID(t, ownerSession, tools.CreatePositionName, map[string]any{
		"id": "p-eng-a", "roleId": "r-eng", "parentId": "p-root",
	})
	invokeExpectID(t, ownerSession, tools.CreatePositionName, map[string]any{
		"id": "p-eng-b", "roleId": "r-eng", "parentId": "p-root",
	})
	invokeExpectID(t, ownerSession, tools.HireWorkerName, map[string]any{
		"id": "w-a", "positionId": "p-eng-a", "kind": "ai",
		"identityContent": "# Alice",
	})
	invokeExpectID(t, ownerSession, tools.HireWorkerName, map[string]any{
		"id": "w-b", "positionId": "p-eng-b", "kind": "ai",
		"identityContent": "# Bob",
	})

	// Sanity: both workers have the v1 content on disk.
	for _, id := range []string{"w-a", "w-b"} {
		data, _ := os.ReadFile(filepath.Join(envsDir, id, "role.md")) //nolint:gosec // path is t.TempDir() + known filename
		if string(data) != "# Engineer v1\nBuild stuff." {
			t.Fatalf("%s pre-update role.md = %q", id, string(data))
		}
	}

	invokeExpectID(t, ownerSession, tools.UpdateRoleName, map[string]any{
		"roleId":  "r-eng",
		"content": "# Engineer v2\nBuild better stuff.",
	})

	// Both workers' role.md should reflect the new content.
	for _, id := range []string{"w-a", "w-b"} {
		data, err := os.ReadFile(filepath.Join(envsDir, id, "role.md")) //nolint:gosec // path is t.TempDir() + known filename
		if err != nil {
			t.Fatalf("read %s role.md: %v", id, err)
		}
		if string(data) != "# Engineer v2\nBuild better stuff." {
			t.Fatalf("%s post-update role.md = %q", id, string(data))
		}
	}
	// And identity.md is untouched.
	for id, want := range map[string]string{"w-a": "# Alice", "w-b": "# Bob"} {
		data, _ := os.ReadFile(filepath.Join(envsDir, id, "identity.md")) //nolint:gosec // path is t.TempDir() + known filename
		if string(data) != want {
			t.Fatalf("%s identity.md = %q, want %q", id, string(data), want)
		}
	}
}

// TestStreamMembers exercises the read-only stream_members tool:
// before any subscriber, members is empty; after a Worker subscribes,
// they appear in the list. This is the "wait until Renée is part of
// the stream" primitive — managers call this before publishing if
// they need to know whether a particular Worker is listening.
func TestStreamMembers(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	envsDir := t.TempDir()

	reg := tools.NewRegistry()
	deps := tools.DefaultDeps(s)
	deps.EnvsDir = envsDir
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.New(s, reg, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := domain.NewRole("r-owner", "# Owner", now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"})
	mustCreate(t, s.Workers.Create(ctx, owner))
	worker, _ := domain.NewAIWorker("w-listener", []domain.PositionID{"p-root"})
	mustCreate(t, s.Workers.Create(ctx, worker))
	for _, name := range []domain.ToolName{
		tools.CreateStreamName,
		tools.StreamMembersName,
		tools.SubscribeName,
	} {
		g, _ := domain.NewToolGrant(domain.GrantID("g-owner-"+name), "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}
	g, _ := domain.NewToolGrant("g-listener-sub", "w-listener", tools.SubscribeName)
	mustCreate(t, s.Grants.Create(ctx, g))

	ownerSession := connectMCP(t, srv.URL, "w-owner")
	listenerSession := connectMCP(t, srv.URL, "w-listener")

	invokeExpectID(t, ownerSession, tools.CreateStreamName, map[string]any{
		"id":   "s-room",
		"name": "room",
	})

	// Empty before anyone subscribes.
	if got := membersOf(t, ownerSession, "s-room"); len(got) != 0 {
		t.Fatalf("members before subscribe = %v, want empty", got)
	}

	invokeOK(t, listenerSession, tools.SubscribeName, map[string]any{"streamId": "s-room"})

	if got := membersOf(t, ownerSession, "s-room"); len(got) != 1 || got[0] != "w-listener" {
		t.Fatalf("members after subscribe = %v, want [w-listener]", got)
	}
}

// TestReadsOverMCP exercises the new read tools: an Owner with the
// full builtin grant set lists workers, lists streams, and reads back
// events on subscribed streams, all over MCP.
func TestReadsOverMCP(t *testing.T) {
	t.Parallel()

	s, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	envsDir := t.TempDir()

	reg := tools.NewRegistry()
	deps := tools.DefaultDeps(s)
	deps.EnvsDir = envsDir
	if err := tools.RegisterBuiltins(reg, deps); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.New(s, reg, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := domain.NewRole("r-owner", "# Owner", now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []domain.PositionID{"p-root"})
	mustCreate(t, s.Workers.Create(ctx, owner))
	for _, name := range []domain.ToolName{
		tools.CreateStreamName,
		tools.SubscribeName,
		tools.PublishName,
		tools.ListWorkersName,
		tools.ListStreamsName,
		tools.ListStreamEventsName,
		tools.ReadEventsName,
	} {
		g, _ := domain.NewToolGrant(domain.GrantID("g-owner-"+name), "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	// Reads work before any state change: list_workers should already see the owner.
	rawWorkers, err := invokeTool(t, ownerSession, tools.ListWorkersName, map[string]any{})
	if err != nil {
		t.Fatalf("list_workers: %v", err)
	}
	var listWorkersOut struct {
		Workers []struct {
			ID string `json:"id"`
		} `json:"workers"`
	}
	if err := json.Unmarshal(rawWorkers, &listWorkersOut); err != nil {
		t.Fatalf("unmarshal list_workers: %v", err)
	}
	if len(listWorkersOut.Workers) != 1 || listWorkersOut.Workers[0].ID != "w-owner" {
		t.Fatalf("list_workers = %+v, want [{w-owner}]", listWorkersOut.Workers)
	}

	// Drive a small mutation through to populate read targets.
	invokeExpectID(t, ownerSession, tools.CreateStreamName, map[string]any{
		"id":   "s-news",
		"name": "news",
	})
	invokeOK(t, ownerSession, tools.SubscribeName, map[string]any{"streamId": "s-news"})
	invokeExpectID(t, ownerSession, tools.PublishName, map[string]any{
		"streamId": "s-news",
		"body":     "first event",
	})

	rawStreams, err := invokeTool(t, ownerSession, tools.ListStreamsName, map[string]any{})
	if err != nil {
		t.Fatalf("list_streams: %v", err)
	}
	var listStreamsOut struct {
		Streams []struct {
			ID string `json:"id"`
		} `json:"streams"`
	}
	if err := json.Unmarshal(rawStreams, &listStreamsOut); err != nil {
		t.Fatalf("unmarshal list_streams: %v", err)
	}
	if len(listStreamsOut.Streams) != 1 || listStreamsOut.Streams[0].ID != "s-news" {
		t.Fatalf("list_streams = %+v, want [{s-news}]", listStreamsOut.Streams)
	}

	rawEvents, err := invokeTool(t, ownerSession, tools.ReadEventsName, map[string]any{})
	if err != nil {
		t.Fatalf("read_events: %v", err)
	}
	var eventsOut struct {
		Events []struct {
			Body string `json:"body"`
		} `json:"events"`
	}
	if err := json.Unmarshal(rawEvents, &eventsOut); err != nil {
		t.Fatalf("unmarshal read_events: %v", err)
	}
	if len(eventsOut.Events) != 1 || eventsOut.Events[0].Body != "first event" {
		t.Fatalf("read_events = %+v, want [{first event}]", eventsOut.Events)
	}
}

func membersOf(t *testing.T, session *mcp.ClientSession, streamID string) []string {
	t.Helper()
	raw, err := invokeTool(t, session, tools.StreamMembersName, map[string]any{"streamId": streamID})
	if err != nil {
		t.Fatalf("stream_members %s: %v", streamID, err)
	}
	var out struct {
		Members []string `json:"members"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	return out.Members
}

// Helpers

func mustCreate(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

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

func invokeTool(t *testing.T, session *mcp.ClientSession, toolName domain.ToolName, args map[string]any) (json.RawMessage, error) {
	t.Helper()
	res, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      string(toolName),
		Arguments: args,
	})
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", toolName, err)
	}
	if res.IsError {
		var detail string
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				detail = tc.Text
			}
		}
		return nil, fmt.Errorf("%s: %s", toolName, detail)
	}
	if len(res.Content) == 0 {
		return nil, fmt.Errorf("%s: empty content", toolName)
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		return nil, fmt.Errorf("%s: content[0] = %T, want *TextContent", toolName, res.Content[0])
	}
	return json.RawMessage(text.Text), nil
}

func invokeExpectID(t *testing.T, session *mcp.ClientSession, toolName domain.ToolName, args map[string]any) string {
	t.Helper()
	result, err := invokeTool(t, session, toolName, args)
	if err != nil {
		t.Fatalf("%s: %v", toolName, err)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	return out.ID
}

// invokeOK is for tools that don't return an `id` (subscribe / unsubscribe).
func invokeOK(t *testing.T, session *mcp.ClientSession, toolName domain.ToolName, args map[string]any) {
	t.Helper()
	if _, err := invokeTool(t, session, toolName, args); err != nil {
		t.Fatalf("%s: %v", toolName, err)
	}
}
