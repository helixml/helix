package api_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// fakeEnsurer captures the ProjectEnsurer.Ensure call and returns the
// seeded IDs. Errors come from `err` if non-nil.
type fakeEnsurer struct {
	mu        sync.Mutex
	calls     int
	lastOrgID string
	lastBid   orgchart.BotID
	projectID string
	agentApp  string
	repoID    string
	err       error
}

func (f *fakeEnsurer) Ensure(_ context.Context, orgID string, bid orgchart.BotID) (string, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastOrgID = orgID
	f.lastBid = bid
	return f.projectID, f.agentApp, f.repoID, f.err
}

// fakeDispatcher records DispatchManual / Dispatch calls so the handler
// test can assert exactly what was enqueued.
type fakeDispatcher struct {
	mu              sync.Mutex
	manualCalls     int
	lastOrgID       string
	lastBotID       orgchart.BotID
	lastActivation  activation.ID
	dispatchedEvent *streaming.Event
}

func (f *fakeDispatcher) Dispatch(_ context.Context, ev streaming.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatchedEvent = &ev
}

func (f *fakeDispatcher) DispatchManual(_ context.Context, orgID string, bid orgchart.BotID, actID activation.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manualCalls++
	f.lastOrgID = orgID
	f.lastBotID = bid
	f.lastActivation = actID
}

// wireActivate rebuilds deps.Activations with the test ensurer +
// dispatcher so the activate use case (which lives in the activations
// service now, not the handler) exercises the fakes.
func wireActivate(deps orgapi.Deps, st *store.Store, ensurer activations.ProjectEnsurer, disp activations.ManualDispatcher) orgapi.Deps {
	deps.Activations = activations.New(activations.Deps{
		Repo:       st.Activations,
		NewID:      func() string { return "act-1" },
		Ensurer:    ensurer,
		Dispatcher: disp,
	})
	return deps
}

// TestActivateBot_HappyPath pins the bug-fix contract: POST
// /bots/{id}/activate runs the ensureProject pipeline (which re-attaches
// the helix-org MCP), enqueues a manual activation, and returns a 202
// carrying the project + agent app IDs plus the pre-allocated activation
// ID. This is what the bot UI's "Start Desktop" button hits instead of
// the generic /sessions/{id}/resume, so the desktop boots with the
// helix-org MCP in Zed's context_servers map.
func TestActivateBot_HappyPath(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")

	ensurer := &fakeEnsurer{projectID: "prj_alice", agentApp: "app_alice", repoID: "repo_alice"}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-alice/activate", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}

	var resp orgapi.BotActivateDTO
	decode(t, rec, &resp)
	if resp.ProjectID != "prj_alice" || resp.AgentAppID != "app_alice" {
		t.Errorf("IDs = (%q,%q), want (prj_alice, app_alice)", resp.ProjectID, resp.AgentAppID)
	}
	if resp.ActivationID == "" {
		t.Errorf("ActivationID must be pre-allocated; got empty")
	}

	if ensurer.calls != 1 {
		t.Errorf("Ensure calls = %d, want 1 (sync ensureProject before enqueue)", ensurer.calls)
	}
	if ensurer.lastBid != "b-alice" || ensurer.lastOrgID != "org-test" {
		t.Errorf("Ensure called with (%q, %q), want (org-test, b-alice)", ensurer.lastOrgID, ensurer.lastBid)
	}

	if disp.manualCalls != 1 {
		t.Errorf("DispatchManual calls = %d, want 1", disp.manualCalls)
	}
	if disp.lastActivation == "" {
		t.Errorf("DispatchManual must carry the pre-allocated activation id; got empty")
	}
	if string(disp.lastActivation) != resp.ActivationID {
		t.Errorf("DispatchManual activation id (%q) does not match response (%q)", disp.lastActivation, resp.ActivationID)
	}

	// Audit row must be persisted with the same ID so the Spawner picks
	// it up and Completes it instead of writing a sibling.
	row, err := st.Activations.Get(ctx, "org-test", activation.ID(resp.ActivationID))
	if err != nil {
		t.Fatalf("activation row missing: %v", err)
	}
	if row.WorkerID != "b-alice" {
		t.Errorf("activation row WorkerID = %q, want b-alice", row.WorkerID)
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerManual {
		t.Errorf("activation row triggers = %+v, want [manual]", row.Triggers)
	}
}

// TestActivateBot_EnsureFailureDoesNotEnqueue pins the synchronous-
// ensureProject contract. If the MCP-attaching step fails, we must
// surface the error to the operator (500) and skip the enqueue —
// queueing an activation against a missing project leaves a dangling
// audit row and confuses the bot's activation timeline.
func TestActivateBot_EnsureFailureDoesNotEnqueue(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")

	ensurer := &fakeEnsurer{err: errors.New("apply project failed")}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots/b-alice/activate", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	if disp.manualCalls != 0 {
		t.Errorf("DispatchManual must not run when ensureProject failed; got %d", disp.manualCalls)
	}

	// No audit row should have been written either — the pre-allocation
	// happens after the ensureProject step.
	rows, err := st.Activations.ListForWorker(ctx, "org-test", "b-alice", 10)
	if err != nil {
		t.Fatalf("list activations: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("activation rows = %d, want 0", len(rows))
	}
}

// TestActivateBot_404OnUnknownBot pins the not-found path so a stale UI
// link returns a useful 404 rather than a 500 from the ensurer.
func TestActivateBot_404OnUnknownBot(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps = wireActivate(deps, st, &fakeEnsurer{}, &fakeDispatcher{})
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots/b-ghost/activate", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

// TestActivateBot_NotImplementedWithoutActivations pins the wiring
// failure mode: if the Activations service is nil in Deps (e.g.
// helix-org disabled by feature flag), the endpoint returns 501 instead
// of nil-derefing.
func TestActivateBot_NotImplementedWithoutActivations(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")
	deps.Activations = nil
	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-alice/activate", nil)
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want 501; body=%s", rec.Code, rec.Body)
	}
}
