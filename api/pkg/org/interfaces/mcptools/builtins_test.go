package mcptools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/roles"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	"github.com/helixml/helix/api/pkg/org/interfaces/server"
)

// TestDemoOwnerHiresCEO walks the "manager does the orchestration" story
// over MCP: each tool does one primitive thing, and the test drives the
// hiring ritual step by step.
//
// Owner is pre-seeded. Owner creates a #general Stream, subscribes
// themselves, defines a CEO Role (markdown content), creates a Position,
// then hires the CEO with an identityContent. The
// Worker's IdentityContent is stored in the domain alongside the Role —
// no env files are written at hire (the spawner projects them at
// activation). Owner publishes; CEO sees it.
func TestDemoOwnerHiresCEO(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// Seed owner directly: role (with the structural tool list), worker, environment.
	now := time.Now().UTC()
	ownerRole, err := orgchart.NewRole(
		"r-owner",
		"# Owner\nBootstrap owner.",
		[]tool.Name{
			mcptools.CreateRoleName,
			mcptools.UpdateRoleName,
			mcptools.HireWorkerName,
			mcptools.CreateStreamName,
			mcptools.SubscribeName,
			mcptools.PublishName,
		},
		nil,
		now,
		"org-test",
	)
	if err != nil {
		t.Fatalf("seed role: %v", err)
	}
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateStreamName, map[string]any{
		"id":   "s-general",
		"name": "general",
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{"streamId": "s-general"})

	invokeExpectID(t, ownerSession, mcptools.CreateRoleName, map[string]any{
		"id":      "r-ceo",
		"content": "# CEO\nLead the company. Subscribe to s-general.",
		"tools":   []string{"publish", "subscribe"},
	})

	invokeExpectID(t, ownerSession, mcptools.HireWorkerName, map[string]any{
		"id":              "w-ceo",
		"roleId":          "r-ceo",
		"parentId":        "w-owner",
		"kind":            "ai",
		"identityContent": "# Meina Gladstone\nCEO. Decisive, warm, direct.",
	})

	// IdentityContent lives in the domain.
	ceoWorker, err := s.Workers.Get(ctx, "org-test", "w-ceo")
	if err != nil {
		t.Fatalf("get w-ceo: %v", err)
	}
	if ceoWorker.IdentityContent() != "# Meina Gladstone\nCEO. Decisive, warm, direct." {
		t.Fatalf("ceo identity = %q", ceoWorker.IdentityContent())
	}

	// hire_worker also creates the transcript and subscribes
	// the hiring Worker's POSITION (not the worker itself) — sub
	// rows are now position-anchored. w-owner is in p-root.
	if _, err := s.Streams.Get(ctx, "org-test", "s-transcript-w-ceo"); err != nil {
		t.Fatalf("transcript missing for w-ceo: %v", err)
	}
	if _, err := s.Subscriptions.Find(ctx, "org-test", "w-owner", "s-transcript-w-ceo"); err != nil {
		t.Fatalf("owner position not subscribed to w-ceo activations: %v", err)
	}
	// The new Worker's own position is intentionally NOT subscribed
	// — otherwise self-published events would loop the dispatcher.
	if _, err := s.Subscriptions.Find(ctx, "org-test", "w-ceo", "s-transcript-w-ceo"); err == nil {
		t.Fatalf("p-ceo should NOT be subscribed to its own worker's transcript")
	}

	// Stand in for the CEO's hire activation: subscribe to the
	// stream they were told about. The dispatcher isn't wired in
	// this test, so we drive it manually. The subscribe tool resolves
	// the caller worker → its position and persists the subscription
	// on the position.
	ceoSession := connectMCP(t, srv.URL, "w-ceo")
	invokeOK(t, ceoSession, mcptools.SubscribeName, map[string]any{"streamId": "s-general"})

	if _, err := s.Subscriptions.Find(ctx, "org-test", "w-ceo", "s-general"); err != nil {
		t.Fatalf("CEO position subscription on s-general missing: %v", err)
	}

	invokeExpectID(t, ownerSession, mcptools.PublishName, map[string]any{
		"streamId": "s-general",
		"body":     "please hire all of your staff",
	})
	ceoEvents, err := s.Events.ListForWorker(ctx, "org-test", "w-ceo", 10)
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

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{
			mcptools.CreateRoleName,
			mcptools.UpdateRoleName,
			mcptools.UpdateIdentityName,
			mcptools.HireWorkerName,
		},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateRoleName, map[string]any{
		"id":      "r-eng",
		"content": "# Engineer v1\nBuild stuff.",
	})
	invokeExpectID(t, ownerSession, mcptools.HireWorkerName, map[string]any{
		"id": "w-a", "roleId": "r-eng", "parentId": "w-owner", "kind": "ai",
		"identityContent": "# Alice",
	})
	invokeExpectID(t, ownerSession, mcptools.HireWorkerName, map[string]any{
		"id": "w-b", "roleId": "r-eng", "parentId": "w-owner", "kind": "ai",
		"identityContent": "# Bob",
	})

	invokeExpectID(t, ownerSession, mcptools.UpdateRoleName, map[string]any{
		"roleId":  "r-eng",
		"content": "# Engineer v2\nBuild better stuff.",
	})

	// Role row in the DB now carries the new content; nothing was
	// written to disk by the tool.
	got, err := s.Roles.Get(ctx, "org-test", "r-eng")
	if err != nil {
		t.Fatalf("get r-eng: %v", err)
	}
	if got.Content != "# Engineer v2\nBuild better stuff." {
		t.Fatalf("r-eng content = %q", got.Content)
	}

	// update_identity rewrites Worker.IdentityContent on the DB row.
	invokeExpectID(t, ownerSession, mcptools.UpdateIdentityName, map[string]any{
		"workerId": "w-a",
		"content":  "# Alice (v2)\nNow with extra spice.",
	})
	wa, err := s.Workers.Get(ctx, "org-test", "w-a")
	if err != nil {
		t.Fatalf("get w-a: %v", err)
	}
	if wa.IdentityContent() != "# Alice (v2)\nNow with extra spice." {
		t.Fatalf("w-a identity = %q", wa.IdentityContent())
	}
	// w-b's identity is untouched.
	wb, err := s.Workers.Get(ctx, "org-test", "w-b")
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

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{mcptools.CreateStreamName, mcptools.StreamMembersName, mcptools.SubscribeName},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	// Subscriptions are position-anchored — give w-listener its own
	// position so a subscribe-by-listener doesn't accidentally
	// subscribe w-owner too. The listener Position also points at its
	// own Role so its MCP surface (just subscribe) is independent of
	// the owner's.
	listenerRole, _ := orgchart.NewRole(
		"r-listener",
		"# Listener",
		[]tool.Name{mcptools.SubscribeName},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, listenerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))
	worker, _ := orgchart.NewAIWorker("w-listener", "r-listener", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, worker))

	ownerSession := connectMCP(t, srv.URL, "w-owner")
	listenerSession := connectMCP(t, srv.URL, "w-listener")

	invokeExpectID(t, ownerSession, mcptools.CreateStreamName, map[string]any{
		"id":   "s-room",
		"name": "room",
	})

	// Empty before anyone subscribes.
	if got := membersOf(t, ownerSession, "s-room"); len(got) != 0 {
		t.Fatalf("members before subscribe = %v, want empty", got)
	}

	invokeOK(t, listenerSession, mcptools.SubscribeName, map[string]any{"streamId": "s-room"})

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

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{mcptools.CreateStreamName, mcptools.InviteWorkersName, mcptools.StreamMembersName},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	// Subscriptions are position-anchored: each worker needs its own
	// position so the invite resolves to per-worker sub rows. With
	// everyone sharing p-root, inviting alice+bob would subscribe
	// p-root once and the owner (also in p-root) would end up listed
	// as a member through that shared subscription. Alice and Bob
	// don't drive MCP calls in this test, so their (empty) Role.Tools
	// is fine.
	memberRole, _ := orgchart.NewRole("r-member", "# Member", nil, nil, now, "org-test")
	mustCreate(t, s.Roles.Create(ctx, memberRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))
	alice, _ := orgchart.NewAIWorker("w-alice", "r-member", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, alice))
	bob, _ := orgchart.NewAIWorker("w-bob", "r-member", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, bob))

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateStreamName, map[string]any{
		"id":   "s-dm",
		"name": "alice ↔ bob",
	})

	// Owner adds both parties to the stream in one call.
	invokeOK(t, ownerSession, mcptools.InviteWorkersName, map[string]any{
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
	invokeOK(t, ownerSession, mcptools.InviteWorkersName, map[string]any{
		"streamId":  "s-dm",
		"workerIds": []string{"w-alice", "w-owner"},
	})
	got = membersOf(t, ownerSession, "s-dm")
	if len(got) != 3 {
		t.Fatalf("members after re-invite = %v, want three", got)
	}

	// Unknown worker -> error, no partial subscription created.
	if _, err := invokeTool(t, ownerSession, mcptools.InviteWorkersName, map[string]any{
		"streamId":  "s-dm",
		"workerIds": []string{"w-ghost"},
	}); err == nil {
		t.Fatalf("inviting unknown worker should error")
	}
	if got = membersOf(t, ownerSession, "s-dm"); len(got) != 3 {
		t.Fatalf("members after failed invite = %v, want three (unchanged)", got)
	}
}

// TestDM exercises the dm tool over MCP. DM channels are provisioned by
// topology for reporting pairs, so the test wires a reporting line
// (Alice manages Bob) and reconciles first; the dm call then publishes
// over the existing per-pair Stream, and the reverse DM reuses it.
func TestDM(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	// Alice and Bob share a Role with both dm + read_events. Because the
	// tool surface is the Role's, both get both, which is fine — Bob
	// simply never calls read_events in this test. Subscriptions are
	// worker-anchored, so the DM subscribes each worker independently.
	memberRole, _ := orgchart.NewRole(
		"r-member",
		"# Member",
		[]tool.Name{mcptools.DMName, mcptools.ReadEventsName},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, memberRole))
	alice, _ := orgchart.NewHumanWorker("w-alice", "r-member", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, alice))
	bob, _ := orgchart.NewAIWorker("w-bob", "r-member", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, bob))

	// DM channels are scoped to reporting relationships: wire one (Alice
	// manages Bob) and reconcile so topology provisions s-dm-w-alice-w-bob
	// before either party DMs the other.
	line, _ := orgchart.NewReportingLine("org-test", "w-alice", "w-bob")
	mustCreate(t, s.ReportingLines.Add(ctx, line))
	if err := deps.Reconciler.Reconcile(ctx, "org-test", "w-bob", "w-alice"); err != nil {
		t.Fatalf("reconcile DM topology: %v", err)
	}

	aliceSession := connectMCP(t, srv.URL, "w-alice")
	bobSession := connectMCP(t, srv.URL, "w-bob")

	// Alice DMs Bob — single call does it all.
	raw, err := invokeTool(t, aliceSession, mcptools.DMName, map[string]any{
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

	// Both positions are subscribed (the DM tool resolves participants
	// → their positions); the event landed in the store.
	for _, pid := range []orgchart.WorkerID{"w-alice", "w-bob"} {
		if _, err := s.Subscriptions.Find(ctx, "org-test", pid, streaming.StreamID(out.StreamID)); err != nil {
			t.Fatalf("%s not subscribed to %s: %v", pid, out.StreamID, err)
		}
	}
	events, _ := s.Events.ListForWorker(ctx, "org-test", "w-bob", 10)
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
	raw, err = invokeTool(t, bobSession, mcptools.DMName, map[string]any{
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
	events, _ = s.Events.ListForWorker(ctx, "org-test", "w-alice", 10)
	if len(events) != 2 {
		t.Fatalf("alice events = %+v, want two", events)
	}

	// Self-DM is rejected up-front.
	if _, err := invokeTool(t, aliceSession, mcptools.DMName, map[string]any{
		"toWorkerId": "w-alice",
		"body":       "hi me",
	}); err == nil {
		t.Fatalf("DM to self should error")
	}
}

// TestReadsOverMCP exercises the read tools: an Owner with the
// full builtin tool set lists workers, lists streams, and reads back
// events on subscribed streams, all over MCP.
func TestReadsOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{
			mcptools.CreateStreamName,
			mcptools.SubscribeName,
			mcptools.PublishName,
			mcptools.ListWorkersName,
			mcptools.ListStreamsName,
			mcptools.ListStreamEventsName,
			mcptools.ReadEventsName,
		},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	// Reads work before any state change: list_workers should already see the owner.
	rawWorkers, err := invokeTool(t, ownerSession, mcptools.ListWorkersName, map[string]any{})
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
	invokeExpectID(t, ownerSession, mcptools.CreateStreamName, map[string]any{
		"id":   "s-news",
		"name": "news",
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{"streamId": "s-news"})
	invokeExpectID(t, ownerSession, mcptools.PublishName, map[string]any{
		"streamId": "s-news",
		"body":     "first event",
	})

	rawStreams, err := invokeTool(t, ownerSession, mcptools.ListStreamsName, map[string]any{})
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

	rawEvents, err := invokeTool(t, ownerSession, mcptools.ReadEventsName, map[string]any{})
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
	raw, err := invokeTool(t, session, mcptools.StreamMembersName, map[string]any{"streamId": streamID})
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

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{mcptools.WorkerLogName},
		nil,
		now,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))
	bot, _ := orgchart.NewAIWorker("w-bot", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, bot))

	// Pre-create the transcript + seed a couple of events. In
	// production hire_worker creates the stream and the spawner
	// publishes events; here we shortcut.
	streamID := streaming.StreamID("s-transcript-w-bot")
	stream, _ := streaming.NewStream(streamID, "Activations: w-bot", "", "w-owner", now, transport.Transport{}, "org-test")
	mustCreate(t, s.Streams.Create(ctx, stream))
	for i, body := range []string{"--- session start ---", "assistant: hello", "=== exit: ok ==="} {
		ev, _ := streaming.NewEvent(
			streaming.EventID(fmt.Sprintf("e-%d", i)),
			streamID,
			"w-bot",
			body,
			now.Add(time.Duration(i)*time.Second),
			"org-test",
		)
		mustCreate(t, s.Events.Append(ctx, ev))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	// First call: returns events newest-first AND auto-subscribes owner.
	raw, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
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
	if _, err := s.Subscriptions.Find(ctx, "org-test", "w-owner", streamID); err != nil {
		t.Fatalf("owner position not subscribed after worker_log: %v", err)
	}

	// since= filters out events at or before the given ID. Pass the
	// middle event's ID; only the newer event ("exit") should remain.
	mid := out.Events[1].ID
	raw, err = invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
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
	if _, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId": "w-ghost",
	}); err == nil {
		t.Fatalf("worker_log on unknown worker should error")
	}

	// Human Worker has no transcript — clear error, not a generic
	// "stream not found".
	if _, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId": "w-owner",
	}); err == nil {
		t.Fatalf("worker_log on human worker should error")
	}
}

// TestWorkerLogFiltersByActivationID pins B5.7 — when activationId is
// passed, worker_log returns only the events that fall inside that
// Activation's [StartedAt, EndedAt] window. Without it, the full
// firehose is returned (back-compat).
func TestWorkerLogFiltersByActivationID(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	ownerRole, _ := orgchart.NewRole(
		"r-owner",
		"# Owner",
		[]tool.Name{mcptools.WorkerLogName},
		nil,
		base,
		"org-test",
	)
	mustCreate(t, s.Roles.Create(ctx, ownerRole))
	owner, _ := orgchart.NewHumanWorker("w-owner", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, owner))
	bot, _ := orgchart.NewAIWorker("w-bot", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, bot))

	streamID := activation.TranscriptID("w-bot")
	str, _ := streaming.NewStream(streamID, "Activations: w-bot", "", "w-owner", base, transport.Transport{}, "org-test")
	mustCreate(t, s.Streams.Create(ctx, str))

	// Two activations, non-overlapping windows. Events at +1s and +2s
	// land inside their respective activation's window.
	a1Start := base
	a1End := base.Add(5 * time.Second)
	a2Start := base.Add(10 * time.Second)
	a2End := base.Add(15 * time.Second)

	a1, _ := activation.New("a-1", "w-bot", []activation.Trigger{{Kind: activation.TriggerHire}}, a1Start, "org-test")
	mustCreate(t, s.Activations.Create(ctx, a1))
	mustCreate(t, s.Activations.Complete(ctx, "org-test", "a-1", activation.Outcome{Status: activation.StatusOK}, a1End))

	a2, _ := activation.New("a-2", "w-bot", []activation.Trigger{{Kind: activation.TriggerEvent}}, a2Start, "org-test")
	mustCreate(t, s.Activations.Create(ctx, a2))
	mustCreate(t, s.Activations.Complete(ctx, "org-test", "a-2", activation.Outcome{Status: activation.StatusOK}, a2End))

	// Seed events spanning both windows + one in the gap (must NOT be
	// returned for either activation_id).
	plan := []struct {
		id   string
		at   time.Time
		body string
	}{
		{"e-1a", a1Start.Add(time.Second), "assistant: a1-first"},
		{"e-1b", a1Start.Add(2 * time.Second), "assistant: a1-second"},
		{"e-gap", a1End.Add(2 * time.Second), "assistant: between"},
		{"e-2a", a2Start.Add(time.Second), "assistant: a2-first"},
		{"e-2b", a2Start.Add(2 * time.Second), "assistant: a2-second"},
	}
	for _, p := range plan {
		ev, _ := streaming.NewEvent(streaming.EventID(p.id), streamID, "w-bot", p.body, p.at, "org-test")
		mustCreate(t, s.Events.Append(ctx, ev))
	}

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	type result struct {
		Events []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"events"`
	}

	// Without activationId: every event on the stream comes back.
	raw, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{"workerId": "w-bot"})
	if err != nil {
		t.Fatalf("worker_log no filter: %v", err)
	}
	var all result
	_ = json.Unmarshal(raw, &all)
	if len(all.Events) != 5 {
		t.Fatalf("no-filter events = %d, want 5 (back-compat)", len(all.Events))
	}

	// activationId=a-1 → only the two events whose CreatedAt falls in
	// [a1Start, a1End]. The gap event and a-2's events are excluded.
	raw, err = invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId":     "w-bot",
		"activationId": "a-1",
	})
	if err != nil {
		t.Fatalf("worker_log a-1: %v", err)
	}
	var first result
	_ = json.Unmarshal(raw, &first)
	if len(first.Events) != 2 {
		t.Fatalf("a-1 events = %d, want 2; got: %+v", len(first.Events), first.Events)
	}
	for _, e := range first.Events {
		if e.ID != "e-1a" && e.ID != "e-1b" {
			t.Errorf("a-1 returned unexpected event %q (%q)", e.ID, e.Body)
		}
	}

	// activationId=a-2 → a-2's two events only.
	raw, err = invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId":     "w-bot",
		"activationId": "a-2",
	})
	if err != nil {
		t.Fatalf("worker_log a-2: %v", err)
	}
	var second result
	_ = json.Unmarshal(raw, &second)
	if len(second.Events) != 2 {
		t.Fatalf("a-2 events = %d, want 2; got: %+v", len(second.Events), second.Events)
	}
	for _, e := range second.Events {
		if e.ID != "e-2a" && e.ID != "e-2b" {
			t.Errorf("a-2 returned unexpected event %q (%q)", e.ID, e.Body)
		}
	}

	// Unknown activationId is a hard error — the caller pointed at a
	// row that doesn't exist, and silently returning [] would be a
	// data-loss bug.
	if _, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId":     "w-bot",
		"activationId": "a-missing",
	}); err == nil {
		t.Fatalf("worker_log with unknown activationId should error")
	}

	// activationId belonging to a *different* Worker is rejected too —
	// no cross-Worker leakage even if the caller knows another
	// Worker's activation IDs.
	other, _ := orgchart.NewAIWorker("w-other", "r-owner", "", "org-test")
	mustCreate(t, s.Workers.Create(ctx, other))
	otherStream, _ := streaming.NewStream(activation.TranscriptID("w-other"), "Activations: w-other", "", "w-owner", base, transport.Transport{}, "org-test")
	mustCreate(t, s.Streams.Create(ctx, otherStream))
	a3, _ := activation.New("a-3", "w-other", []activation.Trigger{{Kind: activation.TriggerHire}}, base, "org-test")
	mustCreate(t, s.Activations.Create(ctx, a3))
	if _, err := invokeTool(t, ownerSession, mcptools.WorkerLogName, map[string]any{
		"workerId":     "w-bot",
		"activationId": "a-3",
	}); err == nil {
		t.Fatalf("worker_log with cross-Worker activationId should error")
	}
}

// seedActingWorker creates a Role (with the baseline read tools injected,
// exactly as the chart's New-Role flow does via the roles service) and a
// human Worker holding it, so a test can connect to the MCP surface AS
// that worker. Replaces the old bootstrap-owner seed now that orgs start
// empty.
func seedActingWorker(t *testing.T, s *store.Store, orgID, workerID, roleID string, tools []tool.Name) {
	t.Helper()
	ctx := context.Background()
	if _, err := roles.New(roles.Deps{Roles: s.Roles, BaseTools: mcptools.BaseReadTools}).
		Create(ctx, orgID, roles.CreateParams{ID: roleID, Content: "# " + roleID, Tools: tools}); err != nil {
		t.Fatalf("seed role %s: %v", roleID, err)
	}
	w, err := orgchart.NewHumanWorker(orgchart.WorkerID(workerID), orgchart.RoleID(roleID), "# "+workerID, orgID)
	if err != nil {
		t.Fatalf("build worker %s: %v", workerID, err)
	}
	if err := s.Workers.Create(ctx, w); err != nil {
		t.Fatalf("create worker %s: %v", workerID, err)
	}
}

// TestWorkerRoleBaselineReadsOverMCP is the §13 regression test for
// helixml/helix#2546: a Worker whose Role was created through the roles
// service (baseline injected) exposes every BaseReadTools entry on its
// MCP surface — most importantly `managers` and `reports`, the two tools
// the QA report observed missing.
func TestWorkerRoleBaselineReadsOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	seedActingWorker(t, s, "org-test", "w-boss", "r-boss", mcptools.OwnerRoleTools())

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	session := connectMCP(t, srv.URL, "w-boss")
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, name := range mcptools.BaseReadTools {
		if !got[name] {
			t.Errorf("baseline tool %q missing from w-boss MCP surface; got: %+v", name, got)
		}
	}
}

// TestCreateRoleInjectsBaselineOverMCP simulates the second half of the
// §13 regression: an owner who creates a role with a minimal mutation-
// only tools list. Without baseline injection, a Worker hired into that
// role would have no way to read its reporting graph. With injection,
// every BaseReadTools entry is on the hired Worker's MCP surface.
func TestCreateRoleInjectsBaselineOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	// Seed a manager worker to act as (the chart's New-Role → Add-Worker
	// entry point, now that there is no auto-seeded owner).
	seedActingWorker(t, s, "org-test", "w-owner", "r-owner", mcptools.OwnerRoleTools())

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ownerSession := connectMCP(t, srv.URL, "w-owner")

	// Owner creates a §13-style QA-engineer role with only the mutation
	// tools its prompt mentions. No reads are listed by the caller —
	// the baseline must arrive via injection.
	invokeExpectID(t, ownerSession, mcptools.CreateRoleName, map[string]any{
		"id":      "r-qa",
		"content": "# QA Engineer\nDM and publish only.",
		"tools":   []string{"dm", "publish", "subscribe", "read_events"},
	})
	invokeExpectID(t, ownerSession, mcptools.HireWorkerName, map[string]any{
		"id":              "w-qa-1",
		"roleId":          "r-qa",
		"parentId":        "w-owner",
		"kind":            "ai",
		"identityContent": "# QA Engineer",
	})

	qaSession := connectMCP(t, srv.URL, "w-qa-1")
	res, err := qaSession.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("qa list tools: %v", err)
	}
	got := make(map[string]bool, len(res.Tools))
	for _, tl := range res.Tools {
		got[tl.Name] = true
	}
	for _, name := range mcptools.BaseReadTools {
		if !got[name] {
			t.Errorf("baseline tool %q missing from w-qa-1 MCP surface; got: %+v", name, got)
		}
	}
	// Caller-supplied tools must also still be present.
	for _, name := range []string{"dm", "publish", "subscribe", "read_events"} {
		if !got[name] {
			t.Errorf("caller-specified tool %q missing from w-qa-1 MCP surface; got: %+v", name, got)
		}
	}
}

// Helpers

func mustCreate(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
}

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
