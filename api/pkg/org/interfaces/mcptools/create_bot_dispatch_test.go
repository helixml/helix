package mcptools

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
)

// fakeDispatcher records DispatchHire / Dispatch calls so the test can
// assert create_bot drives the lifecycle create-dispatch exactly once.
// It satisfies the MCP EventDispatcher (and therefore
// lifecycle.CreateDispatcher via DispatchHire).
type fakeDispatcher struct {
	mu        sync.Mutex
	hires     []dispatchHireCall
	dispatchN int
}

type dispatchHireCall struct {
	orgID        string
	botID        orgchart.BotID
	activationID activation.ID
}

func (f *fakeDispatcher) Dispatch(_ context.Context, _ streaming.Event) {
	f.mu.Lock()
	f.dispatchN++
	f.mu.Unlock()
}

func (f *fakeDispatcher) DispatchHire(_ context.Context, orgID string, botID orgchart.BotID, activationID activation.ID) {
	f.mu.Lock()
	f.hires = append(f.hires, dispatchHireCall{orgID: orgID, botID: botID, activationID: activationID})
	f.mu.Unlock()
}

func (f *fakeDispatcher) hireCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hires)
}

func newCreateBotEnv(t *testing.T) (Config, *fakeDispatcher, tool.Caller) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	now := time.Now().UTC()

	dispatcher := &fakeDispatcher{}
	deps := DefaultDeps(st)
	deps.Dispatcher = dispatcher
	deps.Now = func() time.Time { return now }
	var counter int
	deps.NewID = func() string {
		counter++
		return "id" + string(rune('a'+counter-1))
	}
	caller := botCaller{id: "b-owner", orgID: "org-test"}
	return deps, dispatcher, caller
}

// TestCreateBotReturnsActivationID pins that a successful create
// responds with both the new Bot ID and the create-Activation ID, the
// Activation row exists in the store at response time, and DispatchHire
// fired once with the same id (so the Spawner reuses the row).
func TestCreateBotReturnsActivationID(t *testing.T) {
	t.Parallel()
	deps, dispatcher, caller := newCreateBotEnv(t)
	tl := &CreateBot{deps: deps.Build()}

	args, _ := json.Marshal(createBotArgs{ID: "b-alice", Content: "# Alice"})
	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var resp struct {
		ID           string `json:"id"`
		ActivationID string `json:"activation_id"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	if resp.ID != "b-alice" {
		t.Errorf("response.id = %q, want b-alice", resp.ID)
	}
	if resp.ActivationID == "" {
		t.Fatal("response.activation_id is empty; want a create-Activation ID")
	}

	row, err := deps.Store.Activations.Get(context.Background(), "org-test", activation.ID(resp.ActivationID))
	if err != nil {
		t.Fatalf("activation %q not in store: %v", resp.ActivationID, err)
	}
	if row.WorkerID != "b-alice" {
		t.Errorf("row.WorkerID = %q, want b-alice", row.WorkerID)
	}
	if row.IsCompleted() {
		t.Error("row already completed at create-response time; Spawner hasn't fired yet")
	}
	if n := dispatcher.hireCount(); n != 1 {
		t.Fatalf("DispatchHire calls = %d, want 1", n)
	}
	if got := dispatcher.hires[0].activationID; got != activation.ID(resp.ActivationID) {
		t.Errorf("DispatchHire activationID = %q, want %q", got, resp.ActivationID)
	}
}

// TestCreateBotRequiresContent pins the up-front guard: a create with no
// content is rejected before any row is written.
func TestCreateBotRequiresContent(t *testing.T) {
	t.Parallel()
	deps, _, caller := newCreateBotEnv(t)
	tl := &CreateBot{deps: deps.Build()}

	args, _ := json.Marshal(createBotArgs{ID: "b-empty"})
	_, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if err == nil {
		t.Fatal("expected error for empty content")
	}
	if !strings.Contains(err.Error(), "content") {
		t.Errorf("err = %v, want mention of content", err)
	}
	if _, err := deps.Store.Bots.Get(context.Background(), "org-test", "b-empty"); err == nil {
		t.Fatal("bot row created despite content rejection")
	}
}

// TestCreateBotInheritsCallerOrgID: the new Bot inherits its
// OrganizationID from the creating caller's OrgID.
func TestCreateBotInheritsCallerOrgID(t *testing.T) {
	t.Parallel()
	deps, _, caller := newCreateBotEnv(t)
	tl := &CreateBot{deps: deps.Build()}

	args, _ := json.Marshal(createBotArgs{ID: "b-alice", Content: "# Alice"})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := deps.Store.Bots.Get(context.Background(), "org-test", "b-alice")
	if err != nil {
		t.Fatalf("Get created bot: %v", err)
	}
	if got.OrganizationID != "org-test" {
		t.Errorf("created bot OrgID = %q, want org-test (inherited from caller)", got.OrganizationID)
	}
}
