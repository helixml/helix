package mcptools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/publishing"
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

func injectTestPublishing(cfg *mcptools.Config) {
	deps := publishing.Deps{
		Topics:     cfg.Store.Topics,
		Events:     cfg.Store.Events,
		Dispatcher: cfg.Dispatcher,
		Now:        cfg.Now,
		NewID:      cfg.NewID,
	}
	if cfg.Hub != nil {
		deps.Hub = cfg.Hub
	}
	cfg.Publishing = publishing.New(deps)
}

// TestDemoOwnerCreatesCEO walks the "manager does the orchestration"
// story over MCP: each tool does one primitive thing, and the test
// drives the create ritual step by step.
//
// Owner is pre-seeded. Owner creates a #general Topic, subscribes
// themselves, creates a CEO Bot (markdown content + tools). The CEO Bot
// IS its own job description — there is no separate role/identity, no
// kind. Owner publishes; CEO sees it.
func TestDemoOwnerCreatesCEO(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()

	// Seed owner directly: a Bot with the structural tool list.
	now := time.Now().UTC()
	owner, err := orgchart.NewBot(
		"b-owner",
		"# Owner\nBootstrap owner.",
		[]tool.Name{
			mcptools.CreateBotName,
			mcptools.CreateTopicName,
			mcptools.SubscribeName,
			mcptools.PublishName,
		},
		now,
		"org-test",
	)
	if err != nil {
		t.Fatalf("seed owner bot: %v", err)
	}
	mustCreate(t, s.Bots.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateTopicName, map[string]any{
		"id":   "s-general",
		"name": "general",
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{"botId": "b-owner", "topicIds": []string{"s-general"}})

	// create_bot subscribes the new CEO to s-general at creation — one call,
	// no follow-up subscribe needed (this is the "fewest steps" behavior).
	invokeExpectID(t, ownerSession, mcptools.CreateBotName, map[string]any{
		"id":       "b-ceo",
		"content":  "# CEO\nLead the company.",
		"tools":    []string{"publish", "subscribe"},
		"topics":   []string{"s-general"},
		"parentId": "b-owner",
	})

	if _, err := s.Subscriptions.Find(ctx, "org-test", "b-ceo", "s-general"); err != nil {
		t.Fatalf("CEO subscription on s-general missing: %v", err)
	}

	invokeExpectID(t, ownerSession, mcptools.PublishName, map[string]any{
		"topicId": "s-general",
		"body":    "please hire all of your staff",
	})
	ceoEvents, err := s.Events.ListForBot(ctx, "org-test", "b-ceo", 10)
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

// TestSetBotContentIsDomainWrite pins the post-refactor contract:
// set_bot_content is a pure DB mutation. The spawner is the only thing
// that projects state into envs, so after a tool call the on-disk files
// (if any) are stale and only the DB row reflects the change. Editing
// content preserves the bot's tools.
func TestSetBotContentIsDomainWrite(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	owner, _ := orgchart.NewBot(
		"b-owner",
		"# Owner",
		[]tool.Name{
			mcptools.CreateBotName,
			mcptools.SetBotContentName,
		},
		now,
		"org-test",
	)
	mustCreate(t, s.Bots.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateBotName, map[string]any{
		"id":       "b-eng",
		"content":  "# Engineer v1\nBuild stuff.",
		"tools":    []string{"publish"},
		"topics":   []string{},
		"parentId": "b-owner",
	})

	// The created bot carries publish + the baseline read tools.
	created, err := s.Bots.Get(ctx, "org-test", "b-eng")
	if err != nil {
		t.Fatalf("get b-eng: %v", err)
	}
	toolsBefore := append([]tool.Name(nil), created.Tools...)

	// set_bot_content rewrites Content and preserves Tools.
	invokeExpectID(t, ownerSession, mcptools.SetBotContentName, map[string]any{
		"botId":   "b-eng",
		"content": "# Engineer v2\nBuild better stuff.",
	})

	got, err := s.Bots.Get(ctx, "org-test", "b-eng")
	if err != nil {
		t.Fatalf("get b-eng: %v", err)
	}
	if got.Content != "# Engineer v2\nBuild better stuff." {
		t.Fatalf("b-eng content = %q", got.Content)
	}
	if len(got.Tools) != len(toolsBefore) {
		t.Fatalf("content-only update changed tools: before %v after %v", toolsBefore, got.Tools)
	}
}

// TestTopicMembers exercises the read-only topic_members tool: before
// any subscriber, members is empty; after a Bot subscribes, they appear
// in the list. This is the "wait until Renée is part of the topic"
// primitive — managers call this before publishing if they need to know
// whether a particular Bot is listening.
func TestTopicMembers(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	owner, _ := orgchart.NewBot(
		"b-owner",
		"# Owner",
		[]tool.Name{mcptools.CreateTopicName, mcptools.TopicMembersName, mcptools.SubscribeName},
		now,
		"org-test",
	)
	mustCreate(t, s.Bots.Create(ctx, owner))
	// Subscriptions are bot-anchored — give b-listener its own surface so
	// a subscribe-by-listener doesn't accidentally subscribe b-owner too.
	listener, _ := orgchart.NewBot(
		"b-listener",
		"# Listener",
		[]tool.Name{mcptools.SubscribeName},
		now,
		"org-test",
	)
	mustCreate(t, s.Bots.Create(ctx, listener))

	ownerSession := connectMCP(t, srv.URL, "b-owner")
	listenerSession := connectMCP(t, srv.URL, "b-listener")

	invokeExpectID(t, ownerSession, mcptools.CreateTopicName, map[string]any{
		"id":   "s-room",
		"name": "room",
	})

	// Empty before anyone subscribes.
	if got := membersOf(t, ownerSession, "s-room"); len(got) != 0 {
		t.Fatalf("members before subscribe = %v, want empty", got)
	}

	invokeOK(t, listenerSession, mcptools.SubscribeName, map[string]any{"botId": "b-listener", "topicIds": []string{"s-room"}})

	if got := membersOf(t, ownerSession, "s-room"); len(got) != 1 || got[0] != "b-listener" {
		t.Fatalf("members after subscribe = %v, want [b-listener]", got)
	}
}

// TestSubscribeOtherBots verifies subscribe targets an arbitrary Bot (not
// just the caller): the owner subscribes other bots to a Topic — the
// primitive that lets the initiator open a DM by creating a Topic and
// adding both parties, without requiring the recipient to self-subscribe.
func TestSubscribeOtherBots(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	owner, _ := orgchart.NewBot(
		"b-owner",
		"# Owner",
		[]tool.Name{mcptools.CreateTopicName, mcptools.SubscribeName, mcptools.TopicMembersName},
		now,
		"org-test",
	)
	mustCreate(t, s.Bots.Create(ctx, owner))
	// Subscriptions are bot-anchored: each bot gets its own sub rows.
	alice, _ := orgchart.NewBot("b-alice", "# Alice", nil, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, alice))
	bob, _ := orgchart.NewBot("b-bob", "# Bob", nil, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, bob))

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	invokeExpectID(t, ownerSession, mcptools.CreateTopicName, map[string]any{
		"id":   "s-dm",
		"name": "alice ↔ bob",
	})

	// Owner subscribes both parties to the topic (subscribe targets any bot).
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{
		"botId":    "b-alice",
		"topicIds": []string{"s-dm"},
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{
		"botId":    "b-bob",
		"topicIds": []string{"s-dm"},
	})

	got := membersOf(t, ownerSession, "s-dm")
	if len(got) != 2 {
		t.Fatalf("members after subscribe = %v, want two", got)
	}
	want := map[string]bool{"b-alice": true, "b-bob": true}
	for _, m := range got {
		if !want[m] {
			t.Fatalf("unexpected member %q in %v", m, got)
		}
	}

	// Idempotent: re-subscribing an already-subscribed bot is a no-op.
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{
		"botId":    "b-alice",
		"topicIds": []string{"s-dm"},
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{
		"botId":    "b-owner",
		"topicIds": []string{"s-dm"},
	})
	got = membersOf(t, ownerSession, "s-dm")
	if len(got) != 3 {
		t.Fatalf("members after re-subscribe = %v, want three", got)
	}

	// Unknown bot -> error, no partial subscription created.
	if _, err := invokeTool(t, ownerSession, mcptools.SubscribeName, map[string]any{
		"botId":    "b-ghost",
		"topicIds": []string{"s-dm"},
	}); err == nil {
		t.Fatalf("subscribing unknown bot should error")
	}
	if got = membersOf(t, ownerSession, "s-dm"); len(got) != 3 {
		t.Fatalf("members after failed subscribe = %v, want three (unchanged)", got)
	}
}

// TestDM exercises the dm tool over MCP. DM channels are provisioned by
// topology for reporting pairs, so the test wires a reporting line
// (Alice manages Bob) and reconciles first; the dm call then publishes
// over the existing per-pair Topic, and the reverse DM reuses it.
func TestDM(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	// Alice and Bob both get dm + read_events.
	alice, _ := orgchart.NewBot("b-alice", "# Alice", []tool.Name{mcptools.DMName, mcptools.ReadEventsName}, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, alice))
	bob, _ := orgchart.NewBot("b-bob", "# Bob", []tool.Name{mcptools.DMName, mcptools.ReadEventsName}, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, bob))

	// DM channels are scoped to reporting relationships: wire one (Alice
	// manages Bob) and reconcile so topology provisions s-dm-b-alice-b-bob
	// before either party DMs the other.
	line, _ := orgchart.NewReportingLine("org-test", "b-alice", "b-bob")
	mustCreate(t, s.ReportingLines.Add(ctx, line))
	if err := deps.Reconciler.Reconcile(ctx, "org-test", "b-bob", "b-alice"); err != nil {
		t.Fatalf("reconcile DM topology: %v", err)
	}

	aliceSession := connectMCP(t, srv.URL, "b-alice")
	bobSession := connectMCP(t, srv.URL, "b-bob")

	// Alice DMs Bob — single call does it all.
	raw, err := invokeTool(t, aliceSession, mcptools.DMName, map[string]any{
		"toBotId": "b-bob",
		"body":    "hey",
	})
	if err != nil {
		t.Fatalf("dm: %v", err)
	}
	var out struct {
		ID      string `json:"id"`
		TopicID string `json:"topicId"`
		To      string `json:"to"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal dm: %v", err)
	}
	if out.TopicID != "s-dm-b-alice-b-bob" {
		t.Fatalf("topicId = %q, want s-dm-b-alice-b-bob", out.TopicID)
	}
	if out.To != "b-bob" {
		t.Fatalf("to = %q, want b-bob", out.To)
	}

	// Both bots are subscribed (the DM tool resolves participants); the
	// event landed in the store.
	for _, bid := range []orgchart.BotID{"b-alice", "b-bob"} {
		if _, err := s.Subscriptions.Find(ctx, "org-test", bid, streaming.TopicID(out.TopicID)); err != nil {
			t.Fatalf("%s not subscribed to %s: %v", bid, out.TopicID, err)
		}
	}
	events, _ := s.Events.ListForBot(ctx, "org-test", "b-bob", 10)
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
	if msg.From != "b-alice" || len(msg.To) != 1 || msg.To[0] != "b-bob" {
		t.Fatalf("dm envelope = %+v, want from=b-alice to=[b-bob]", msg)
	}

	// Bob replies. Reverse direction reuses the same Topic — the IDs are
	// sorted, so A→B and B→A share one ordered conversation.
	raw, err = invokeTool(t, bobSession, mcptools.DMName, map[string]any{
		"toBotId": "b-alice",
		"body":    "hi back",
	})
	if err != nil {
		t.Fatalf("reply dm: %v", err)
	}
	var reply struct {
		TopicID string `json:"topicId"`
	}
	_ = json.Unmarshal(raw, &reply)
	if reply.TopicID != out.TopicID {
		t.Fatalf("reply topicId = %q, want %q (DM topic should be reused)", reply.TopicID, out.TopicID)
	}

	// Alice can read the conversation through her own subscription.
	events, _ = s.Events.ListForBot(ctx, "org-test", "b-alice", 10)
	if len(events) != 2 {
		t.Fatalf("alice events = %+v, want two", events)
	}

	// Self-DM is rejected up-front.
	if _, err := invokeTool(t, aliceSession, mcptools.DMName, map[string]any{
		"toBotId": "b-alice",
		"body":    "hi me",
	}); err == nil {
		t.Fatalf("DM to self should error")
	}
}

// TestReadsOverMCP exercises the read tools: an Owner with the full
// builtin tool set lists bots, lists topics, and reads back events on
// subscribed topics, all over MCP.
func TestReadsOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()
	owner, _ := orgchart.NewBot(
		"b-owner",
		"# Owner",
		[]tool.Name{
			mcptools.CreateTopicName,
			mcptools.SubscribeName,
			mcptools.PublishName,
			mcptools.ListBotsName,
			mcptools.ListTopicsName,
			mcptools.ListTopicEventsName,
			mcptools.ReadEventsName,
		},
		now,
		"org-test",
	)
	mustCreate(t, s.Bots.Create(ctx, owner))

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	// Reads work before any state change: list_bots should already see the owner.
	rawBots, err := invokeTool(t, ownerSession, mcptools.ListBotsName, map[string]any{})
	if err != nil {
		t.Fatalf("list_bots: %v", err)
	}
	var listBotsOut struct {
		Bots []struct {
			ID string `json:"id"`
		} `json:"bots"`
	}
	if err := json.Unmarshal(rawBots, &listBotsOut); err != nil {
		t.Fatalf("unmarshal list_bots: %v", err)
	}
	if len(listBotsOut.Bots) != 1 || listBotsOut.Bots[0].ID != "b-owner" {
		t.Fatalf("list_bots = %+v, want [{b-owner}]", listBotsOut.Bots)
	}

	// Drive a small mutation through to populate read targets.
	invokeExpectID(t, ownerSession, mcptools.CreateTopicName, map[string]any{
		"id":   "s-news",
		"name": "news",
	})
	invokeOK(t, ownerSession, mcptools.SubscribeName, map[string]any{"botId": "b-owner", "topicIds": []string{"s-news"}})
	invokeExpectID(t, ownerSession, mcptools.PublishName, map[string]any{
		"topicId": "s-news",
		"body":    "first event",
	})

	rawTopics, err := invokeTool(t, ownerSession, mcptools.ListTopicsName, map[string]any{})
	if err != nil {
		t.Fatalf("list_topics: %v", err)
	}
	var listTopicsOut struct {
		Topics []struct {
			ID string `json:"id"`
		} `json:"topics"`
	}
	if err := json.Unmarshal(rawTopics, &listTopicsOut); err != nil {
		t.Fatalf("unmarshal list_topics: %v", err)
	}
	if len(listTopicsOut.Topics) != 1 || listTopicsOut.Topics[0].ID != "s-news" {
		t.Fatalf("list_topics = %+v, want [{s-news}]", listTopicsOut.Topics)
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

func membersOf(t *testing.T, session *mcp.ClientSession, topicID string) []string {
	t.Helper()
	raw, err := invokeTool(t, session, mcptools.TopicMembersName, map[string]any{"topicId": topicID})
	if err != nil {
		t.Fatalf("topic_members %s: %v", topicID, err)
	}
	var out struct {
		Members []string `json:"members"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal members: %v", err)
	}
	return out.Members
}

// TestBotLog covers the bot_log shortcut: read a Bot's activation
// transcript by botId without having to know the topic naming
// convention. The first call auto-subscribes the caller; later calls are
// pure reads. since/limit semantics mirror read_events.
func TestBotLog(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	now := time.Now().UTC()

	owner, _ := orgchart.NewBot("b-owner", "# Owner", []tool.Name{mcptools.BotLogName}, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, owner))
	bot, _ := orgchart.NewBot("b-bot", "# Bot", nil, now, "org-test")
	mustCreate(t, s.Bots.Create(ctx, bot))

	// Pre-create the transcript + seed a couple of events. In production
	// create_bot creates the topic and the spawner publishes events; here
	// we shortcut.
	topicID := activation.TranscriptID("b-bot")
	topic, _ := streaming.NewTopic(topicID, "Activations: b-bot", "", "b-owner", now, transport.Transport{}, "org-test")
	mustCreate(t, s.Topics.Create(ctx, topic))
	for i, body := range []string{"--- session start ---", "assistant: hello", "=== exit: ok ==="} {
		ev, _ := streaming.NewEvent(
			streaming.EventID(fmt.Sprintf("e-%d", i)),
			topicID,
			"b-bot",
			body,
			now.Add(time.Duration(i)*time.Second),
			"org-test",
		)
		mustCreate(t, s.Events.Append(ctx, ev))
	}

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	// First call: returns events newest-first AND auto-subscribes owner.
	raw, err := invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId": "b-bot",
	})
	if err != nil {
		t.Fatalf("bot_log: %v", err)
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
	if _, err := s.Subscriptions.Find(ctx, "org-test", "b-owner", topicID); err != nil {
		t.Fatalf("owner not subscribed after bot_log: %v", err)
	}

	// since= filters out events at or before the given ID. Pass the
	// middle event's ID; only the newer event ("exit") should remain.
	mid := out.Events[1].ID
	raw, err = invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId": "b-bot",
		"since": mid,
	})
	if err != nil {
		t.Fatalf("bot_log since: %v", err)
	}
	_ = json.Unmarshal(raw, &out)
	if len(out.Events) != 1 || out.Events[0].Body != "=== exit: ok ===" {
		t.Fatalf("since-filtered = %+v, want exit only", out.Events)
	}

	// Unknown bot errors with a clear message.
	if _, err := invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId": "b-ghost",
	}); err == nil {
		t.Fatalf("bot_log on unknown bot should error")
	}
}

// TestBotLogFiltersByActivationID pins B5.7 — when activationId is
// passed, bot_log returns only the events that fall inside that
// Activation's [StartedAt, EndedAt] window. Without it, the full
// firehose is returned (back-compat).
func TestBotLogFiltersByActivationID(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ctx := context.Background()
	base := time.Now().UTC().Truncate(time.Second)

	owner, _ := orgchart.NewBot("b-owner", "# Owner", []tool.Name{mcptools.BotLogName}, base, "org-test")
	mustCreate(t, s.Bots.Create(ctx, owner))
	bot, _ := orgchart.NewBot("b-bot", "# Bot", nil, base, "org-test")
	mustCreate(t, s.Bots.Create(ctx, bot))

	topicID := activation.TranscriptID("b-bot")
	str, _ := streaming.NewTopic(topicID, "Activations: b-bot", "", "b-owner", base, transport.Transport{}, "org-test")
	mustCreate(t, s.Topics.Create(ctx, str))

	// Two activations, non-overlapping windows. Events at +1s and +2s
	// land inside their respective activation's window.
	a1Start := base
	a1End := base.Add(5 * time.Second)
	a2Start := base.Add(10 * time.Second)
	a2End := base.Add(15 * time.Second)

	a1, _ := activation.New("a-1", "b-bot", []activation.Trigger{{Kind: activation.TriggerHire}}, a1Start, "org-test")
	mustCreate(t, s.Activations.Create(ctx, a1))
	mustCreate(t, s.Activations.Complete(ctx, "org-test", "a-1", activation.Outcome{Status: activation.StatusOK}, a1End))

	a2, _ := activation.New("a-2", "b-bot", []activation.Trigger{{Kind: activation.TriggerEvent}}, a2Start, "org-test")
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
		ev, _ := streaming.NewEvent(streaming.EventID(p.id), topicID, "b-bot", p.body, p.at, "org-test")
		mustCreate(t, s.Events.Append(ctx, ev))
	}

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	type result struct {
		Events []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"events"`
	}

	// Without activationId: every event on the topic comes back.
	raw, err := invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{"botId": "b-bot"})
	if err != nil {
		t.Fatalf("bot_log no filter: %v", err)
	}
	var all result
	_ = json.Unmarshal(raw, &all)
	if len(all.Events) != 5 {
		t.Fatalf("no-filter events = %d, want 5 (back-compat)", len(all.Events))
	}

	// activationId=a-1 → only the two events whose CreatedAt falls in
	// [a1Start, a1End]. The gap event and a-2's events are excluded.
	raw, err = invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId":        "b-bot",
		"activationId": "a-1",
	})
	if err != nil {
		t.Fatalf("bot_log a-1: %v", err)
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
	raw, err = invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId":        "b-bot",
		"activationId": "a-2",
	})
	if err != nil {
		t.Fatalf("bot_log a-2: %v", err)
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

	// Unknown activationId is a hard error — the caller pointed at a row
	// that doesn't exist, and silently returning [] would be a data-loss
	// bug.
	if _, err := invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId":        "b-bot",
		"activationId": "a-missing",
	}); err == nil {
		t.Fatalf("bot_log with unknown activationId should error")
	}

	// activationId belonging to a *different* Bot is rejected too — no
	// cross-Bot leakage even if the caller knows another Bot's activation
	// IDs.
	other, _ := orgchart.NewBot("b-other", "# Other", nil, base, "org-test")
	mustCreate(t, s.Bots.Create(ctx, other))
	otherTopic, _ := streaming.NewTopic(activation.TranscriptID("b-other"), "Activations: b-other", "", "b-owner", base, transport.Transport{}, "org-test")
	mustCreate(t, s.Topics.Create(ctx, otherTopic))
	a3, _ := activation.New("a-3", "b-other", []activation.Trigger{{Kind: activation.TriggerHire}}, base, "org-test")
	mustCreate(t, s.Activations.Create(ctx, a3))
	if _, err := invokeTool(t, ownerSession, mcptools.BotLogName, map[string]any{
		"botId":        "b-bot",
		"activationId": "a-3",
	}); err == nil {
		t.Fatalf("bot_log with cross-Bot activationId should error")
	}
}

// seedActingBot creates a Bot (with the baseline read tools injected,
// exactly as the chart's create-bot flow does via the bots service) so a
// test can connect to the MCP surface AS that bot.
func seedActingBot(t *testing.T, s *store.Store, orgID, botID string, tools []tool.Name) {
	t.Helper()
	ctx := context.Background()
	if _, err := bots.New(bots.Deps{Bots: s.Bots, BaseTools: mcptools.BaseReadTools}).
		Create(ctx, orgID, bots.CreateParams{ID: botID, Content: "# " + botID, Tools: tools}); err != nil {
		t.Fatalf("seed bot %s: %v", botID, err)
	}
}

// TestBotBaselineReadsOverMCP is the §13 regression test for
// helixml/helix#2546: a Bot created through the bots service (baseline
// injected) exposes every BaseReadTools entry on its MCP surface — most
// importantly `managers` and `reports`, the two tools the QA report
// observed missing.
func TestBotBaselineReadsOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	seedActingBot(t, s, "org-test", "b-boss", mcptools.OwnerBotTools())

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	session := connectMCP(t, srv.URL, "b-boss")
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
			t.Errorf("baseline tool %q missing from b-boss MCP surface; got: %+v", name, got)
		}
	}
}

// TestCreateBotInjectsBaselineOverMCP simulates the second half of the
// §13 regression: an owner who creates a bot with a minimal mutation-
// only tools list. Without baseline injection, that bot would have no
// way to read its reporting graph. With injection, every BaseReadTools
// entry is on the created Bot's MCP surface.
func TestCreateBotInjectsBaselineOverMCP(t *testing.T) {
	t.Parallel()

	s := orggorm.GetOrgTestDB(t)
	// Seed a manager bot to act as.
	seedActingBot(t, s, "org-test", "b-owner", mcptools.OwnerBotTools())

	reg := mcptools.NewRegistry()
	deps := mcptools.DefaultDeps(s)
	injectTestPublishing(&deps)
	if err := mcptools.RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	srv := httptest.NewServer(server.NewFromStore(s, reg, nil, nil, nil).Handler())
	t.Cleanup(srv.Close)

	ownerSession := connectMCP(t, srv.URL, "b-owner")

	// Owner creates a §13-style QA-engineer bot with only the mutation
	// tools its prompt mentions. No reads are listed by the caller — the
	// baseline must arrive via injection.
	invokeExpectID(t, ownerSession, mcptools.CreateBotName, map[string]any{
		"id":       "b-qa-1",
		"content":  "# QA Engineer\nDM and publish only.",
		"tools":    []string{"dm", "publish", "subscribe", "read_events"},
		"parentId": "b-owner",
	})

	qaSession := connectMCP(t, srv.URL, "b-qa-1")
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
			t.Errorf("baseline tool %q missing from b-qa-1 MCP surface; got: %+v", name, got)
		}
	}
	// Caller-supplied tools must also still be present.
	for _, name := range []string{"dm", "publish", "subscribe", "read_events"} {
		if !got[name] {
			t.Errorf("caller-specified tool %q missing from b-qa-1 MCP surface; got: %+v", name, got)
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
