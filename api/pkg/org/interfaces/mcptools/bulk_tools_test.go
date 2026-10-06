package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
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
	n := 0
	deps.NewID = func() string {
		n++
		return fmt.Sprintf("fixed-%d", n)
	}
	injectTestPublishing(&deps)
	reg := NewRegistry()
	// The catalogue is read lazily, so wiring it before RegisterBuiltins
	// is what the composition root does too.
	deps.KnownTools = Catalogue(reg)
	if err := RegisterBuiltins(reg, deps.Build()); err != nil {
		t.Fatalf("register builtins: %v", err)
	}
	owner, err := orgchart.NewNode("b-owner", "# Owner", nil, deps.Now(), "org-test")
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	if err := st.Nodes.Create(context.Background(), owner); err != nil {
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
	b, err := orgchart.NewNode(orgchart.NodeID(id), "# "+id, nil, time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC), "org-test")
	if err != nil {
		t.Fatalf("new bot %q: %v", id, err)
	}
	if err := st.Nodes.Create(context.Background(), b); err != nil {
		t.Fatalf("create bot %q: %v", id, err)
	}
}

func seedTriggerRow(t *testing.T, st *store.Store, id string) {
	t.Helper()
	row, err := trigger.New(id, "org-test", id, "", transport.KindLocal, nil, "b-owner",
		time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("new trigger %q: %v", id, err)
	}
	if err := st.Triggers.Create(context.Background(), row); err != nil {
		t.Fatalf("create trigger %q: %v", id, err)
	}
}

// attached reports whether the Worker has an attachment to the Trigger.
func attached(t *testing.T, st *store.Store, worker orgchart.NodeID, triggerID string) bool {
	t.Helper()
	rows, err := st.WorkerAttachments.Find(context.Background(), store.WithOrg("org-test"),
		store.WithWorkerID(worker), store.WithTriggerID(triggerID), store.WithLimit(1))
	if err != nil {
		t.Fatalf("find attachment: %v", err)
	}
	return len(rows) > 0
}

func toolSet(b orgchart.Node) map[tool.Name]bool {
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
		"botId": "b-eng", "tools": []string{ChatName, AttachWorkerName},
	}); err != nil {
		t.Fatalf("attach: %v", err)
	}
	got, _ := st.Nodes.Get(ctx, "org-test", "b-eng")
	ts := toolSet(got)
	if !ts[ChatName] || !ts[AttachWorkerName] {
		t.Fatalf("attach did not add tools: %v", got.Tools)
	}

	// Idempotent re-attach: same tool set, no error.
	if _, err := invoke(t, reg, caller, AttachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{ChatName},
	}); err != nil {
		t.Fatalf("re-attach: %v", err)
	}

	// Detach a subset.
	if _, err := invoke(t, reg, caller, DetachToolName, map[string]any{
		"botId": "b-eng", "tools": []string{ChatName},
	}); err != nil {
		t.Fatalf("detach: %v", err)
	}
	got, _ = st.Nodes.Get(ctx, "org-test", "b-eng")
	if toolSet(got)[ChatName] {
		t.Fatalf("detach did not remove publish: %v", got.Tools)
	}
	if !toolSet(got)[AttachWorkerName] {
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

func TestToolChangeNotifiesLinkedAgentOnce(t *testing.T) {
	st := orggorm.GetOrgTestDB(t)
	deps := DefaultDeps(st)
	var notified []string
	deps.ToolChangeNotifier = func(_ context.Context, appID string) {
		notified = append(notified, appID)
	}
	bot, err := orgchart.NewNode("b-eng", "# Engineer", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(context.Background(), bot.WithAgentID("app-eng")); err != nil {
		t.Fatal(err)
	}

	if _, err := deps.Build().Nodes.AttachTools(context.Background(), "org-test", "b-eng", []tool.Name{ChatName}); err != nil {
		t.Fatal(err)
	}
	if len(notified) != 1 || notified[0] != "app-eng" {
		t.Fatalf("notifications = %v, want [app-eng]", notified)
	}
}

// TestCreateBotAttachesToTriggers pins the "fewest steps" behavior:
// Triggers listed at creation become real attachment rows, and an
// unknown Trigger fails the whole create with no partial bot.
func TestCreateBotAttachesToTriggers(t *testing.T) {
	t.Parallel()
	st, reg, caller := bulkTestEnv(t)
	ctx := context.Background()
	seedTriggerRow(t, st, "s-a")
	seedTriggerRow(t, st, "s-b")

	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-ceo", "content": "# CEO",
		"tools": []string{ChatName}, "triggers": []string{"s-a", "s-b"},
	}); err != nil {
		t.Fatalf("create_bot: %v", err)
	}
	for _, tid := range []string{"s-a", "s-b"} {
		if !attached(t, st, "b-ceo", tid) {
			t.Fatalf("expected attachment (b-ceo, %s)", tid)
		}
	}

	// Unknown Trigger -> error and no bot row created.
	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-ghost", "content": "# Ghost", "tools": []string{}, "triggers": []string{"s-nope"},
	}); err == nil {
		t.Fatalf("create with unknown trigger should error")
	}
	if _, err := st.Nodes.Get(ctx, "org-test", "b-ghost"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("failed create left a partial bot row: %v", err)
	}

	// Unknown tool name rejected.
	if _, err := invoke(t, reg, caller, CreateBotName, map[string]any{
		"id": "b-x", "content": "# X", "tools": []string{"no_such_tool"}, "triggers": []string{},
	}); err == nil {
		t.Fatalf("create with unknown tool should error")
	}
}

// TestDeleteBotCascades verifies delete_bot removes the bot and its
// attachments, and errors on an absent bot.
func TestDeleteBotCascades(t *testing.T) {
	t.Parallel()
	st, reg, caller := bulkTestEnv(t)
	ctx := context.Background()
	seedTriggerRow(t, st, "s-a")
	seedBotRow(t, st, "b-eng")
	if _, err := invoke(t, reg, caller, AttachWorkerName, map[string]any{
		"botId": "b-eng", "triggerIds": []string{"s-a"},
	}); err != nil {
		t.Fatalf("seed attachment: %v", err)
	}

	if _, err := invoke(t, reg, caller, DeleteBotName, map[string]any{"botId": "b-eng"}); err != nil {
		t.Fatalf("delete_bot: %v", err)
	}
	if _, err := st.Nodes.Get(ctx, "org-test", "b-eng"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("bot still present after delete: %v", err)
	}
	if attached(t, st, "b-eng", "s-a") {
		t.Fatal("attachment not cascaded on delete")
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

	assertArrayProp := func(name tool.Name, prop string, wantEnum, wantRequired bool) {
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
		if found != wantRequired {
			t.Fatalf("%s.%s: required=%v, want %v (required=%v)", name, prop, found, wantRequired, s.Required)
		}
	}

	assertArrayProp(AttachToolName, "tools", true, true)
	assertArrayProp(DetachToolName, "tools", true, true)
	assertArrayProp(CreateBotName, "tools", true, true)
	assertArrayProp(CreateBotName, "triggers", false, true)
	// attach/detach take either triggerIds or processorOutputs, so
	// neither is individually required — the tool rejects "both empty".
	assertArrayProp(AttachWorkerName, "triggerIds", false, false)
	assertArrayProp(AttachWorkerName, "processorOutputs", false, false)
	assertArrayProp(DetachWorkerName, "triggerIds", false, false)
	assertArrayProp(DetachWorkerName, "processorOutputs", false, false)

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
