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

	"github.com/helixml/helix/api/pkg/org/event"
	"github.com/helixml/helix/api/pkg/org/grant"
	"github.com/helixml/helix/api/pkg/org/position"
	"github.com/helixml/helix/api/pkg/org/role"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/tool"
	"github.com/helixml/helix/api/pkg/org/transport"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/server"
	"github.com/helixml/helix/helix-org/store/sqlite"
	"github.com/helixml/helix/helix-org/tools"
)

// TestDemoOwnerHiresCEO walks the "manager does the orchestration" story
// over MCP: each tool does one primitive thing, and the test drives the
// hiring ritual step by step.
//
// Owner is pre-seeded. Owner creates a #general Stream, subscribes
// themselves, defines a CEO Role (markdown content), creates a Position,
// then hires the CEO with inline grants and an identityContent. The
// Worker's IdentityContent is stored in the domain alongside the Role —
// no env files are written at hire (the spawner projects them at
// activation). Owner publishes; CEO sees it.
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// Seed owner directly: role, position, worker, environment, structural grants.
	now := time.Now().UTC()
	ownerRole, err := role.New("r-owner", "# Owner\nBootstrap owner.", nil, nil, now)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	ownerEnvPath := filepath.Join(envsDir, "w-owner")
	if err := os.MkdirAll(ownerEnvPath, 0o750); err != nil {
		t.Fatalf("mkdir owner env: %v", err)
	}
	ownerEnv, _ := domain.NewEnvironment("w-owner", ownerEnvPath, now)
	mustCreate(t, s.Environments.Create(ctx, ownerEnv))
	for _, name := range []tool.Name{
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.CreatePositionName,
		tools.HireWorkerName,
		tools.GrantToolName,
		tools.CreateStreamName,
		tools.SubscribeName,
		tools.PublishName,
	} {
		grantID := grant.ID("g-owner-" + name)
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

	// hire_worker creates the env directory but does not write files —
	// the spawner projects role.md / identity.md / agent.md at
	// activation time. So we should see the directory exist but be
	// empty after a hire.
	ceoEnvPath := filepath.Join(envsDir, "w-ceo")
	if entries, err := os.ReadDir(ceoEnvPath); err != nil {
		t.Fatalf("expected env dir to exist: %v", err)
	} else if len(entries) != 0 {
		t.Fatalf("expected empty env dir, got %d entries", len(entries))
	}
	// IdentityContent lives in the domain.
	ceoWorker, err := s.Workers.Get(ctx, "w-ceo")
	if err != nil {
		t.Fatalf("get w-ceo: %v", err)
	}
	if ceoWorker.IdentityContent() != "# Meina Gladstone\nCEO. Decisive, warm, direct." {
		t.Fatalf("ceo identity = %q", ceoWorker.IdentityContent())
	}

	// hire_worker also creates the activation stream and subscribes the
	// hiring Worker (the owner) so they can audit by reading events.
	if _, err := s.Streams.Get(ctx, "s-activations-w-ceo"); err != nil {
		t.Fatalf("activation stream missing for w-ceo: %v", err)
	}
	if _, err := s.Subscriptions.Find(ctx, "w-owner", "s-activations-w-ceo"); err != nil {
		t.Fatalf("owner not subscribed to w-ceo activations: %v", err)
	}
	// The new Worker themselves is intentionally NOT subscribed —
	// otherwise self-published events would loop the dispatcher.
	if _, err := s.Subscriptions.Find(ctx, "w-ceo", "s-activations-w-ceo"); err == nil {
		t.Fatalf("w-ceo should NOT be subscribed to its own activation stream")
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
	if len(ceoEvents) != 1 {
		t.Fatalf("ceo events = %+v, want 1", ceoEvents)
	}
	msg, err := ceoEvents[0].Message()
	if err != nil {
		t.Fatalf("parse ceo event message: %v", err)
	}
	if msg.Body != "please hire all of your staff" {
		t.Fatalf("ceo event body = %q", msg.Body)
	}
}

// TestUpdateRoleAndIdentityAreDomainWrites pins the post-refactor
// contract: update_role and update_identity are pure DB mutations.
// The spawner is the only thing that projects state into envs, so
// after a tool call the on-disk files (if any) are stale and only
// the DB row reflects the change. This test hires two workers and
// asserts that both `update_role` and `update_identity` flow through
// the domain alone — no fan-out walks, no cross-env writes.
func TestUpdateRoleAndIdentityAreDomainWrites(t *testing.T) {
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	for _, name := range []tool.Name{
		tools.CreateRoleName,
		tools.UpdateRoleName,
		tools.UpdateIdentityName,
		tools.CreatePositionName,
		tools.HireWorkerName,
	} {
		g, _ := domain.NewToolGrant(grant.ID("g-"+name), "w-owner", name)
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

	// hire_worker does not write env files; the dirs exist but are empty.
	for _, id := range []string{"w-a", "w-b"} {
		entries, err := os.ReadDir(filepath.Join(envsDir, id))
		if err != nil {
			t.Fatalf("read %s env dir: %v", id, err)
		}
		if len(entries) != 0 {
			t.Fatalf("%s env should be empty after hire, got %d entries", id, len(entries))
		}
	}

	invokeExpectID(t, ownerSession, tools.UpdateRoleName, map[string]any{
		"roleId":  "r-eng",
		"content": "# Engineer v2\nBuild better stuff.",
	})

	// Role row in the DB now carries the new content; nothing was
	// written to disk by the tool.
	got, err := s.Roles.Get(ctx, "r-eng")
	if err != nil {
		t.Fatalf("get r-eng: %v", err)
	}
	if got.Content != "# Engineer v2\nBuild better stuff." {
		t.Fatalf("r-eng content = %q", got.Content)
	}
	for _, id := range []string{"w-a", "w-b"} {
		entries, _ := os.ReadDir(filepath.Join(envsDir, id))
		if len(entries) != 0 {
			t.Fatalf("%s env should still be empty after update_role, got %d entries", id, len(entries))
		}
	}

	// update_identity rewrites Worker.IdentityContent on the DB row.
	invokeExpectID(t, ownerSession, tools.UpdateIdentityName, map[string]any{
		"workerId": "w-a",
		"content":  "# Alice (v2)\nNow with extra spice.",
	})
	wa, err := s.Workers.Get(ctx, "w-a")
	if err != nil {
		t.Fatalf("get w-a: %v", err)
	}
	if wa.IdentityContent() != "# Alice (v2)\nNow with extra spice." {
		t.Fatalf("w-a identity = %q", wa.IdentityContent())
	}
	// w-b's identity is untouched.
	wb, err := s.Workers.Get(ctx, "w-b")
	if err != nil {
		t.Fatalf("get w-b: %v", err)
	}
	if wb.IdentityContent() != "# Bob" {
		t.Fatalf("w-b identity changed: %q", wb.IdentityContent())
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	worker, _ := domain.NewAIWorker("w-listener", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, worker))
	for _, name := range []tool.Name{
		tools.CreateStreamName,
		tools.StreamMembersName,
		tools.SubscribeName,
	} {
		g, _ := domain.NewToolGrant(grant.ID("g-owner-"+name), "w-owner", name)
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

// TestInviteWorkers verifies one Worker can subscribe others to a
// Stream — the primitive that lets the initiator open a DM by creating
// a Stream and adding both parties, without requiring the recipient to
// self-subscribe first.
func TestInviteWorkers(t *testing.T) {
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	alice, _ := domain.NewAIWorker("w-alice", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, alice))
	bob, _ := domain.NewAIWorker("w-bob", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, bob))
	for _, name := range []tool.Name{
		tools.CreateStreamName,
		tools.InviteWorkersName,
		tools.StreamMembersName,
	} {
		g, _ := domain.NewToolGrant(grant.ID("g-owner-"+name), "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, tools.CreateStreamName, map[string]any{
		"id":   "s-dm",
		"name": "alice ↔ bob",
	})

	// Owner adds both parties to the stream in one call.
	invokeOK(t, ownerSession, tools.InviteWorkersName, map[string]any{
		"streamId":  "s-dm",
		"workerIds": []string{"w-alice", "w-bob"},
	})

	got := membersOf(t, ownerSession, "s-dm")
	if len(got) != 2 {
		t.Fatalf("members after invite = %v, want two", got)
	}
	want := map[string]bool{"w-alice": true, "w-bob": true}
	for _, m := range got {
		if !want[m] {
			t.Fatalf("unexpected member %q in %v", m, got)
		}
	}

	// Idempotent: re-inviting an already-subscribed worker alongside a
	// new one is a no-op for the existing subscription and a success
	// for the rest.
	invokeOK(t, ownerSession, tools.InviteWorkersName, map[string]any{
		"streamId":  "s-dm",
		"workerIds": []string{"w-alice", "w-owner"},
	})
	got = membersOf(t, ownerSession, "s-dm")
	if len(got) != 3 {
		t.Fatalf("members after re-invite = %v, want three", got)
	}

	// Unknown worker -> error, no partial subscription created.
	if _, err := invokeTool(t, ownerSession, tools.InviteWorkersName, map[string]any{
		"streamId":  "s-dm",
		"workerIds": []string{"w-ghost"},
	}); err == nil {
		t.Fatalf("inviting unknown worker should error")
	}
	if got = membersOf(t, ownerSession, "s-dm"); len(got) != 3 {
		t.Fatalf("members after failed invite = %v, want three (unchanged)", got)
	}
}

// TestDM exercises the dm tool: a single call from Alice to Bob
// creates the per-pair Stream, subscribes both, and publishes the
// body. A second DM in the reverse direction reuses the same Stream.
func TestDM(t *testing.T) {
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	alice, _ := domain.NewHumanWorker("w-alice", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, alice))
	bob, _ := domain.NewAIWorker("w-bob", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, bob))
	for _, name := range []tool.Name{tools.DMName, tools.ReadEventsName} {
		g, _ := domain.NewToolGrant(grant.ID("g-alice-"+name), "w-alice", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}
	bobDMGrant, _ := domain.NewToolGrant("g-bob-dm", "w-bob", tools.DMName)
	mustCreate(t, s.Grants.Create(ctx, bobDMGrant))

	aliceSession := connectMCP(t, srv.URL, "w-alice")
	bobSession := connectMCP(t, srv.URL, "w-bob")

	// Alice DMs Bob — single call does it all.
	raw, err := invokeTool(t, aliceSession, tools.DMName, map[string]any{
		"toWorkerId": "w-bob",
		"body":       "hey",
	})
	if err != nil {
		t.Fatalf("dm: %v", err)
	}
	var out struct {
		ID       string `json:"id"`
		StreamID string `json:"streamId"`
		To       string `json:"to"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal dm: %v", err)
	}
	if out.StreamID != "s-dm-w-alice-w-bob" {
		t.Fatalf("streamId = %q, want s-dm-w-alice-w-bob", out.StreamID)
	}
	if out.To != "w-bob" {
		t.Fatalf("to = %q, want w-bob", out.To)
	}

	// Both parties are subscribed; the event landed in the store.
	for _, wid := range []worker.ID{"w-alice", "w-bob"} {
		if _, err := s.Subscriptions.Find(ctx, wid, stream.ID(out.StreamID)); err != nil {
			t.Fatalf("%s not subscribed to %s: %v", wid, out.StreamID, err)
		}
	}
	events, _ := s.Events.ListForWorker(ctx, "w-bob", 10)
	if len(events) != 1 {
		t.Fatalf("bob events = %+v, want one", events)
	}
	msg, err := events[0].Message()
	if err != nil {
		t.Fatalf("parse dm event: %v", err)
	}
	if msg.Body != "hey" {
		t.Fatalf("dm body = %q, want hey", msg.Body)
	}
	if msg.From != "w-alice" || len(msg.To) != 1 || msg.To[0] != "w-bob" {
		t.Fatalf("dm envelope = %+v, want from=w-alice to=[w-bob]", msg)
	}

	// Bob replies. Reverse direction reuses the same Stream — the IDs
	// are sorted, so A→B and B→A share one ordered conversation.
	raw, err = invokeTool(t, bobSession, tools.DMName, map[string]any{
		"toWorkerId": "w-alice",
		"body":       "hi back",
	})
	if err != nil {
		t.Fatalf("reply dm: %v", err)
	}
	var reply struct {
		StreamID string `json:"streamId"`
	}
	_ = json.Unmarshal(raw, &reply)
	if reply.StreamID != out.StreamID {
		t.Fatalf("reply streamId = %q, want %q (DM stream should be reused)", reply.StreamID, out.StreamID)
	}

	// Alice can read the conversation through her own subscription.
	events, _ = s.Events.ListForWorker(ctx, "w-alice", 10)
	if len(events) != 2 {
		t.Fatalf("alice events = %+v, want two", events)
	}

	// Self-DM is rejected up-front.
	if _, err := invokeTool(t, aliceSession, tools.DMName, map[string]any{
		"toWorkerId": "w-alice",
		"body":       "hi me",
	}); err == nil {
		t.Fatalf("DM to self should error")
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	for _, name := range []tool.Name{
		tools.CreateStreamName,
		tools.SubscribeName,
		tools.PublishName,
		tools.ListWorkersName,
		tools.ListStreamsName,
		tools.ListStreamEventsName,
		tools.ReadEventsName,
	} {
		g, _ := domain.NewToolGrant(grant.ID("g-owner-"+name), "w-owner", name)
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

// TestWorkerLog covers the worker_log shortcut: read a Worker's
// activation transcript by workerId without having to know the stream
// naming convention. The first call auto-subscribes the caller; later
// calls are pure reads. since/limit semantics mirror read_events.
func TestWorkerLog(t *testing.T) {
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
	srv := httptest.NewServer(server.New(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := role.New("r-owner", "# Owner", nil, nil, now)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	mustCreate(t, s.Positions.Create(ctx, rootPos))
	owner, _ := domain.NewHumanWorker("w-owner", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, owner))
	bot, _ := domain.NewAIWorker("w-bot", []position.ID{"p-root"}, "")
	mustCreate(t, s.Workers.Create(ctx, bot))

	// Pre-create the activation stream + seed a couple of events. In
	// production hire_worker creates the stream and the spawner
	// publishes events; here we shortcut.
	streamID := stream.ID("s-activations-w-bot")
	stream, _ := domain.NewStream(streamID, "Activations: w-bot", "", "w-owner", now, transport.Transport{})
	mustCreate(t, s.Streams.Create(ctx, stream))
	for i, body := range []string{"--- session start ---", "assistant: hello", "=== exit: ok ==="} {
		ev, _ := domain.NewEvent(
			event.ID(fmt.Sprintf("e-%d", i)),
			streamID,
			"w-bot",
			body,
			now.Add(time.Duration(i)*time.Second),
		)
		mustCreate(t, s.Events.Append(ctx, ev))
	}

	for _, name := range []tool.Name{tools.WorkerLogName} {
		g, _ := domain.NewToolGrant(grant.ID("g-owner-"+name), "w-owner", name)
		mustCreate(t, s.Grants.Create(ctx, g))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	// First call: returns events newest-first AND auto-subscribes owner.
	raw, err := invokeTool(t, ownerSession, tools.WorkerLogName, map[string]any{
		"workerId": "w-bot",
	})
	if err != nil {
		t.Fatalf("worker_log: %v", err)
	}
	var out struct {
		Events []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(out.Events))
	}
	// Newest first.
	if out.Events[0].Body != "=== exit: ok ===" {
		t.Fatalf("newest = %q, want exit marker", out.Events[0].Body)
	}
	if _, err := s.Subscriptions.Find(ctx, "w-owner", streamID); err != nil {
		t.Fatalf("owner not subscribed after worker_log: %v", err)
	}

	// since= filters out events at or before the given ID. Pass the
	// middle event's ID; only the newer event ("exit") should remain.
	mid := out.Events[1].ID
	raw, err = invokeTool(t, ownerSession, tools.WorkerLogName, map[string]any{
		"workerId": "w-bot",
		"since":    mid,
	})
	if err != nil {
		t.Fatalf("worker_log since: %v", err)
	}
	_ = json.Unmarshal(raw, &out)
	if len(out.Events) != 1 || out.Events[0].Body != "=== exit: ok ===" {
		t.Fatalf("since-filtered = %+v, want exit only", out.Events)
	}

	// Unknown worker errors with a clear message.
	if _, err := invokeTool(t, ownerSession, tools.WorkerLogName, map[string]any{
		"workerId": "w-ghost",
	}); err == nil {
		t.Fatalf("worker_log on unknown worker should error")
	}

	// Human Worker has no activation stream — clear error, not a generic
	// "stream not found".
	if _, err := invokeTool(t, ownerSession, tools.WorkerLogName, map[string]any{
		"workerId": "w-owner",
	}); err == nil {
		t.Fatalf("worker_log on human worker should error")
	}
}

// Helpers

func mustCreate(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

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

func invokeTool(t *testing.T, session *mcp.ClientSession, toolName tool.Name, args map[string]any) (json.RawMessage, error) {
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

func invokeExpectID(t *testing.T, session *mcp.ClientSession, toolName tool.Name, args map[string]any) string {
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
func invokeOK(t *testing.T, session *mcp.ClientSession, toolName tool.Name, args map[string]any) {
	t.Helper()
	if _, err := invokeTool(t, session, toolName, args); err != nil {
		t.Fatalf("%s: %v", toolName, err)
	}
}
