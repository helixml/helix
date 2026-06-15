package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// fakeEnsurer captures the ProjectEnsurer.Ensure call and returns
// the seeded IDs. Errors come from `err` if non-nil.
type fakeEnsurer struct {
	mu        sync.Mutex
	calls     int
	lastOrgID string
	lastWid   orgchart.WorkerID
	projectID string
	agentApp  string
	repoID    string
	err       error
}

func (f *fakeEnsurer) Ensure(_ context.Context, orgID string, wid orgchart.WorkerID) (string, string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastOrgID = orgID
	f.lastWid = wid
	return f.projectID, f.agentApp, f.repoID, f.err
}

// fakeDispatcher records DispatchManual / Dispatch calls so the
// handler test can assert exactly what was enqueued.
type fakeDispatcher struct {
	mu              sync.Mutex
	manualCalls     int
	lastOrgID       string
	lastWorkerID    orgchart.WorkerID
	lastActivation  activation.ID
	dispatchedEvent *streaming.Event
}

func (f *fakeDispatcher) Dispatch(_ context.Context, ev streaming.Event) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.dispatchedEvent = &ev
}

func (f *fakeDispatcher) DispatchManual(_ context.Context, orgID string, wid orgchart.WorkerID, actID activation.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.manualCalls++
	f.lastOrgID = orgID
	f.lastWorkerID = wid
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

// TestActivateWorker_HappyPath pins the bug-fix contract: POST
// /workers/{id}/activate runs the ensureProject pipeline (which
// re-attaches the helix-org MCP), enqueues a manual activation, and
// returns a 202 carrying the project + agent app IDs plus the
// pre-allocated activation ID. This is what the worker UI's
// "Start Desktop" button hits instead of the generic
// /sessions/{id}/resume, so that the desktop boots with the
// helix-org MCP in Zed's context_servers map.
func TestActivateWorker_HappyPath(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "alice identity")

	ensurer := &fakeEnsurer{projectID: "prj_alice", agentApp: "app_alice", repoID: "repo_alice"}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/workers/w-alice/activate", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}

	var resp orgapi.WorkerActivateDTO
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
	if ensurer.lastWid != "w-alice" || ensurer.lastOrgID != "org-test" {
		t.Errorf("Ensure called with (%q, %q), want (org-test, w-alice)", ensurer.lastOrgID, ensurer.lastWid)
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

	// Audit row must be persisted with the same ID so the Spawner
	// picks it up and Completes it instead of writing a sibling.
	row, err := st.Activations.Get(ctx, "org-test", activation.ID(resp.ActivationID))
	if err != nil {
		t.Fatalf("activation row missing: %v", err)
	}
	if row.WorkerID != "w-alice" {
		t.Errorf("activation row WorkerID = %q, want w-alice", row.WorkerID)
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerManual {
		t.Errorf("activation row triggers = %+v, want [manual]", row.Triggers)
	}
}

// TestActivateWorker_AllowsHumanWorker pins the no-kind-gating
// contract: every identity in helix-org (owner + hired humans + AI
// workers) has a chat agent and a desktop, and Restart Desktop must
// work uniformly across all of them. The endpoint runs the same
// pipeline regardless of kind — ensureProject (which re-attaches the
// helix-org MCP) and a TriggerManual enqueue.
func TestActivateWorker_AllowsHumanWorker(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedOwnerPosition(t, st, ctx)
	human, err := orgchart.NewHumanWorker("w-human", "r-owner", "human identity", "org-test")
	if err != nil {
		t.Fatalf("NewHumanWorker: %v", err)
	}
	if err := st.Workers.Create(ctx, human); err != nil {
		t.Fatalf("create human worker: %v", err)
	}

	ensurer := &fakeEnsurer{projectID: "prj_human", agentApp: "app_human"}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/workers/w-human/activate", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	if ensurer.calls != 1 {
		t.Errorf("Ensure must run for human worker; got %d calls", ensurer.calls)
	}
	if disp.manualCalls != 1 {
		t.Errorf("DispatchManual must run for human worker; got %d calls", disp.manualCalls)
	}
}

// TestActivateWorker_EnsureFailureDoesNotEnqueue pins the synchronous-
// ensureProject contract. If the MCP-attaching step fails, we must
// surface the error to the operator (500) and skip the enqueue —
// queueing an activation against a missing project leaves a dangling
// audit row and confuses the worker's activation timeline.
func TestActivateWorker_EnsureFailureDoesNotEnqueue(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "alice identity")

	ensurer := &fakeEnsurer{err: errors.New("apply project failed")}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, ensurer, disp)
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/workers/w-alice/activate", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	if disp.manualCalls != 0 {
		t.Errorf("DispatchManual must not run when ensureProject failed; got %d", disp.manualCalls)
	}

	// No audit row should have been written either — the
	// pre-allocation happens after the ensureProject step.
	rows, err := st.Activations.ListForWorker(ctx, "org-test", "w-alice", 10)
	if err != nil {
		t.Fatalf("list activations: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("activation rows = %d, want 0", len(rows))
	}
}

// TestActivateWorker_404OnUnknownWorker pins the not-found path so a
// stale UI link returns a useful 404 rather than a 500 from the
// ensurer (the ProjectEnsurer needs a worker row to do its work).
func TestActivateWorker_404OnUnknownWorker(t *testing.T) {
	deps, _, _ := newDeps(t)
	deps.ProjectEnsurer = &fakeEnsurer{}
	deps.Dispatcher = &fakeDispatcher{}
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/workers/w-ghost/activate", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}

// TestActivateWorker_NotImplementedWithoutDeps pins the wiring
// failure mode: if either ProjectEnsurer or Dispatcher is nil in
// Deps (e.g. helix-org disabled by feature flag), the endpoint
// returns 501 instead of nil-derefing.
func TestActivateWorker_NotImplementedWithoutDeps(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup func(d *orgapi.Deps)
	}{
		{"no ProjectEnsurer", func(d *orgapi.Deps) { d.Dispatcher = &fakeDispatcher{} }},
		{"no Dispatcher", func(d *orgapi.Deps) { d.ProjectEnsurer = &fakeEnsurer{} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			deps, st, _ := newDeps(t)
			ctx := context.Background()
			seedOwnerPosition(t, st, ctx)
			mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "")
			tc.setup(&deps)
			h := orgapi.Handler(deps)
			rec := do(t, h, "POST", "/workers/w-alice/activate", nil)
			if rec.Code != http.StatusNotImplemented {
				t.Fatalf("status = %d, want 501; body=%s", rec.Code, rec.Body)
			}
		})
	}
}

// silence unused-import warnings if a later test removes the only
// reference to one of these.
var (
	_ = json.Marshal
	_ = httptest.NewRecorder
	_ = store.ErrNotFound
)
