package mcptools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/publishing"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/domain/trigger"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// injectTestPublishing wires the real publish use case onto a test
// Config, so tool tests exercise the same append→notify→route path
// production does.
func injectTestPublishing(cfg *Config) {
	deps := publishing.Deps{
		Triggers: cfg.Store.Triggers,
		Events:   cfg.Store.Events,
		Now:      cfg.Now,
		NewID:    cfg.NewID,
	}
	if cfg.Dispatcher != nil {
		deps.Router = cfg.Dispatcher
	}
	if cfg.Hub != nil {
		deps.Hub = cfg.Hub
	}
	cfg.Publishing = publishing.New(deps)
}

func TestRegisterBuiltinsRequiresPublishing(t *testing.T) {
	deps := DefaultDeps(orggorm.GetOrgTestDB(t)).Build()
	err := RegisterBuiltins(NewRegistry(), deps)
	if err == nil || !strings.Contains(err.Error(), "deps.Publishing is required") {
		t.Fatalf("RegisterBuiltins error = %v", err)
	}
}

// seedChatTrigger creates a Trigger of the given kind and a caller
// Worker allowed to use `chat`.
func seedChatTrigger(t *testing.T, st *store.Store, id string, kind transport.Kind, config []byte) botCaller {
	t.Helper()
	ctx := context.Background()
	now := time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC)
	row, err := trigger.New(id, "org-test", id, "", kind, config, "b-worker", now)
	if err != nil {
		t.Fatalf("new trigger: %v", err)
	}
	if err := st.Triggers.Create(ctx, row); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	w, err := orgchart.NewNode("b-worker", "# Worker", []tool.Name{ChatName}, now, "org-test")
	if err != nil {
		t.Fatalf("new worker: %v", err)
	}
	if err := st.Nodes.Create(ctx, w); err != nil && !errors.Is(err, store.ErrConflict) {
		t.Fatalf("create worker: %v", err)
	}
	return botCaller{id: "b-worker", orgID: "org-test"}
}

func chatTool(t *testing.T, st *store.Store) *Chat {
	t.Helper()
	cfg := DefaultDeps(st)
	cfg.Now = func() time.Time { return time.Date(2026, 6, 10, 0, 0, 0, 0, time.UTC) }
	cfg.NewID = func() string { return "fixed" }
	injectTestPublishing(&cfg)
	return &Chat{deps: cfg.Build()}
}

// TestChatRejectsInboundTrigger: `chat` is internal-only. A Trigger
// backed by an external transport is inbound-only, and the tool says so
// rather than silently doing nothing — acting on the provider is the
// Worker's job with get_secret and that provider's API.
func TestChatRejectsInboundTrigger(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	caller := seedChatTrigger(t, st, "s-gh", transport.KindGitHub, []byte(`{"repo":"helixml/helix","events":["issues"]}`))

	args, _ := json.Marshal(map[string]any{"triggerId": "s-gh", "body": "hello"})
	_, err := chatTool(t, st).Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if !errors.Is(err, publishing.ErrNotAnInternalChannel) {
		t.Fatalf("err = %v, want ErrNotAnInternalChannel", err)
	}
	if !strings.Contains(err.Error(), "get_secret") {
		t.Fatalf("error should point at the real route: %v", err)
	}
}

// TestChatSendsToInternalChannel: a local Trigger is an internal
// channel, which is exactly what `chat` addresses.
func TestChatSendsToInternalChannel(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	caller := seedChatTrigger(t, st, "s-team-b-worker", transport.KindLocal, nil)

	args, _ := json.Marshal(map[string]any{"triggerId": "s-team-b-worker", "body": "standup at 10"})
	raw, err := chatTool(t, st).Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	var got struct {
		ID        string `json:"id"`
		TriggerID string `json:"triggerId"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.ID == "" || got.TriggerID != "s-team-b-worker" {
		t.Fatalf("chat result = %+v", got)
	}

	events, err := st.Events.ListForStream(context.Background(), "org-test", "s-team-b-worker", 10)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	msg, _ := events[0].Message()
	// Attribution is the caller's, not whatever the args claimed.
	if msg.From != "b-worker" || msg.Body != "standup at 10" {
		t.Fatalf("message = %+v", msg)
	}
}

// TestChatRequiresArgs pins the cheap input guards.
func TestChatRequiresArgs(t *testing.T) {
	t.Parallel()
	st := orggorm.GetOrgTestDB(t)
	caller := seedChatTrigger(t, st, "s-room", transport.KindLocal, nil)
	tl := chatTool(t, st)

	for _, args := range []map[string]any{
		{"body": "no trigger"},
		{"triggerId": "s-room"},
	} {
		raw, _ := json.Marshal(args)
		if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: raw}); err == nil {
			t.Fatalf("args %v should be rejected", args)
		}
	}
}
