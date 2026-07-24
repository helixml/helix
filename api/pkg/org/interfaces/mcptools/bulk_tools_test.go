package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// bulkTestEnv wires a store + registry with the ToolNames provider and
// lifecycle in place (RegisterBuiltins does the wiring), plus a seeded
// owner caller. Returns everything a bulk-tools test needs.
func bulkTestEnv(t *testing.T) (*store.Store, *Registry, botCaller) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	deps := DefaultDeps(st)
	deps.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	deps.NewID = func() string { return "fixed" }
	injectTestPublishing(&deps)
	reg := NewRegistry()
	if err := RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	owner, err := orgchart.NewBot("b-owner", "# Owner", nil, deps.Now(), "org-test")
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := st.Bots.Create(context.Background(), owner); err != nil {
		t.Fatalf("create owner: %v", err)
	}
	return st, reg, botCaller{id: "b-owner", orgID: "org-test"}
}

func invoke(t *testing.T, reg *Registry, caller botCaller, name tool.Name, args map[string]any) (json.RawMessage, error) {
	t.Helper()
	tl, err := reg.Get(name)
	if err != nil {
		t.Fatalf("get tool %q: %v", name, err)
	}
	raw, _ := json.Marshal(args)
	return tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: raw})
}

func seedBotRow(t *testing.T, st *store.Store, id string) {
	t.Helper()
	b, err := orgchart.NewBot(orgchart.BotID(id), "# "+id, nil, time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), "org-test")
	if err != nil {
		t.Fatalf("new bot %q: %v", id, err)
	}
	if err := st.Bots.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot %q: %v", id, err)
	}
}

func seedTopicRow(t *testing.T, st *store.Store, id string) {
	t.Helper()
	tp, err := streaming.NewTopic(streaming.TopicID(id), id, "", "b-owner",
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), transport.LocalTransport(), "org-test")
	if err != nil {
		t.Fatalf("new topic %q: %v", id, err)
	}
	if err := st.Topics.Create(context.Background(), tp); err != nil {
		t.Fatalf("create topic %q: %v", id, err)
	}
}

func toolSet(b orgchart.Bot) map[tool.Name]bool {
	m := make(map[tool.Name]bool, len(b.Tools))
	for _, n := range b.Tools {
		m[n] = true
	}
	return m
}

// TestAttachDetachTools covers the bulk grant/revoke happy path plus the
// guards: idempotency, baseline protection, and unknown-tool rejection.
func TestAttachDetachTools(t *testing.T) {
	t.Parallel()
	st, reg, caller := bulkTestEnv(t)
	ctx := context.Background()
	seedBotRow(t, st, "b-eng")

	// Attach a multi-tool array.
	if _, err := invoke(t, reg, caller, AttachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{PublishName, SubscribeName},
	}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, _ := st.Bots.Get(ctx, "org-test", "b-eng")
	ts := toolSet(got)
	if !ts[PublishName] || !ts[SubscribeName] {
		t.Fatalf("attach did not add tools: %v", got.Tools)
	}

	// Idempotent re-attach: same tool set, no error.
	if _, err := invoke(t, reg, caller, AttachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{PublishName},
	}); err != nil {
		t.Fatalf("re-attach: %v", err)
	}

	// Detach a subset.
	if _, err := invoke(t, reg, caller, DetachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{PublishName},
	}); err != nil {
		t.Fatalf("detach: %v", err)
	}
	got, _ = st.Bots.Get(ctx, "org-test", "b-eng")
	if toolSet(got)[PublishName] {
		t.Fatalf("detach did not remove publish: %v", got.Tools)
	}
	if !toolSet(got)[SubscribeName] {
		t.Fatalf("detach removed too much: %v", got.Tools)
	}

	// Detach refuses a baseline tool.
	if _, err := invoke(t, reg, caller, DetachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{GetBotName},
	}); err == nil {
		t.Fatalf("detaching a baseline tool should error")
	}

	// Unknown tool name rejected on attach (whole call fails).
	if _, err := invoke(t, reg, caller, AttachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{"no_such_tool"},
	}); err == nil {
		t.Fatalf("attaching an unknown tool should error")
	}
}

// TestCreateBotSubscribesToTopics pins the "fewest steps" behavior: topics
// listed at creation become real subscription rows, and an unknown topic
// fails the whole create with no partial bot.
func TestCreateBotSubscribesToTopics(t *testing.T) {
	t.Parallel()
	st, reg, caller := bulkTestEnv(t)
	ctx := context.Background()
	seedTopicRow(t, st, "s-a")
	seedTopicRow(t, st, "s-b")

	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-ceo", "content": "# CEO",
		"tools": []string{PublishName}, "topics": []string{"s-a", "s-b"},
	}); err != nil {
		t.Fatalf("create_bot: %v", err)
	}
	for _, tid := range []streaming.TopicID{"s-a", "s-b"} {
		if _, err := st.Subscriptions.Find(ctx, "org-test", "b-ceo", tid); err != nil {
			t.Fatalf("expected subscription (b-ceo, %s): %v", tid, err)
		}
	}

	// Unknown topic -> error and no bot row created (validated up front).
	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-ghost", "content": "# Ghost", "tools": []string{}, "topics": []string{"s-nope"},
	}); err == nil {
		t.Fatalf("create with unknown topic should error")
	}
	if _, err := st.Bots.Get(ctx, "org-test", "b-ghost"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed create left a partial bot row: %v", err)
	}

	// Unknown tool name rejected.
	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-x", "content": "# X", "tools": []string{"no_such_tool"}, "topics": []string{},
	}); err == nil {
		t.Fatalf("create with unknown tool should error")
	}
}

// TestDeleteBotCascades verifies delete_bot removes the bot and its
// subscriptions, and errors on an absent bot.
func TestDeleteBotCascades(t *testing.T) {
	t.Parallel()
	st, reg, caller := bulkTestEnv(t)
	ctx := context.Background()
	seedTopicRow(t, st, "s-a")
	seedBotRow(t, st, "b-eng")
	sub, err := streaming.NewSubscription("b-eng", "s-a", time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), "org-test")
	if err != nil {
		t.Fatalf("new subscription: %v", err)
	}
	if err := st.Subscriptions.Create(ctx, sub); err != nil {
		t.Fatalf("seed subscription: %v", err)
	}

	if _, err := invoke(t, reg, caller, DeleteBotName, map[string]any{"botId": "b-eng"}); err != nil {
		t.Fatalf("delete_bot: %v", err)
	}
	if _, err := st.Bots.Get(ctx, "org-test", "b-eng"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bot still present after delete: %v", err)
	}
	if _, err := st.Subscriptions.Find(ctx, "org-test", "b-eng", "s-a"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("subscription not cascaded on delete: %v", err)
	}

	// Absent bot -> error.
	if _, err := invoke(t, reg, caller, DeleteBotName, map[string]any{"botId": "b-missing"}); err == nil {
		t.Fatalf("deleting an absent bot should error")
	}
}

// TestBulkToolSchemas asserts the advertised schemas are non-nullable
// arrays (no ["null","array"] union) with the tool-name enum on tool
// arguments — the whole point of the schema fix.
func TestBulkToolSchemas(t *testing.T) {
	t.Parallel()
	_, reg, _ := bulkTestEnv(t)

	assertArrayProp := func(name tool.Name, prop string, wantEnum bool) {
		tl, err := reg.Get(name)
		if err != nil {
			t.Fatalf("get %q: %v", name, err)
		}
		s := tl.InputSchema()
		p := s.Properties[prop]
		if p == nil {
			t.Fatalf("%s: missing property %q", name, prop)
		}
		if p.Type != "array" || len(p.Types) != 0 {
			t.Fatalf("%s.%s: type=%q types=%v, want a plain \"array\" (no null union)", name, prop, p.Type, p.Types)
		}
		if p.Items == nil {
			t.Fatalf("%s.%s: missing items", name, prop)
		}
		if wantEnum && len(p.Items.Enum) == 0 {
			t.Fatalf("%s.%s: items has no enum, want the tool-name catalogue", name, prop)
		}
		if !wantEnum && len(p.Items.Enum) != 0 {
			t.Fatalf("%s.%s: items unexpectedly has an enum", name, prop)
		}
		found := false
		for _, r := range s.Required {
			if r == prop {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s.%s: not in required %v", name, prop, s.Required)
		}
	}

	assertArrayProp(AttachToolName, "tools", true)
	assertArrayProp(DetachToolName, "tools", true)
	assertArrayProp(CreateBotName, "tools", true)
	assertArrayProp(CreateBotName, "topics", false)
	assertArrayProp(SubscribeName, "topicIds", false)
	assertArrayProp(UnsubscribeName, "topicIds", false)

	// The create_bot tools enum reflects the live registry (create_bot itself
	// is registered, so it must appear among the valid names).
	cb, _ := reg.Get(CreateBotName)
	enum := cb.InputSchema().Properties["tools"].Items.Enum
	var sawCreateBot bool
	for _, e := range enum {
		if e == string(CreateBotName) {
			sawCreateBot = true
		}
	}
	if !sawCreateBot {
		t.Fatalf("tools enum %v does not include registered tool %q", enum, CreateBotName)
	}
}
