package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// fakeWorkerRuntime returns a fixed session id for the worker so the
// restart handler can decide between "recreate existing container" and
// "fall back to a fresh activation".
type fakeWorkerRuntime struct {
	sessionID string
}

func (f fakeWorkerRuntime) State(_ context.Context, _ string, _ orgchart.WorkerID) (orgapi.WorkerRuntimeInfo, error) {
	return orgapi.WorkerRuntimeInfo{SessionID: f.sessionID}, nil
}

// fakeRestarter records RestartSession calls so the test can assert the
// worker-page button funnels into the shared backend restart primitive
// rather than the activate → SendMessage path.
type fakeRestarter struct {
	mu      sync.Mutex
	calls   int
	lastSID string
	err     error
}

func (f *fakeRestarter) RestartSession(_ context.Context, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastSID = sessionID
	return f.err
}

// TestRestartWorkerAgent_RecreatesExistingSession pins the fix: when the
// worker has a live session, the worker-page "Restart agent session"
// button recreates that session's container via the SessionRestarter
// port (the shared backend primitive) — it does NOT enqueue a normal
// activation (which would SendMessage to the still-stuck container).
func TestRestartWorkerAgent_RecreatesExistingSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-alice", "p-root", "alice identity")

	restarter := &fakeRestarter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_alice"}, disp)
	deps.WorkerRuntime = fakeWorkerRuntime{sessionID: "ses_alice"}
	deps.SessionRestarter = restarter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/workers/w-alice/restart-agent", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}

	var resp orgapi.WorkerActivateDTO
	decode(t, rec, &resp)
	if resp.SessionID != "ses_alice" {
		t.Errorf("SessionID = %q, want ses_alice", resp.SessionID)
	}
	if restarter.calls != 1 || restarter.lastSID != "ses_alice" {
		t.Errorf("RestartSession calls = %d lastSID = %q, want 1 / ses_alice", restarter.calls, restarter.lastSID)
	}
	if disp.manualCalls != 0 {
		t.Errorf("DispatchManual must NOT run when a live session is restarted; got %d", disp.manualCalls)
	}
}

// TestRestartWorkerAgent_FallsBackToActivateWhenNoSession pins the
// first-time path: a worker with no live session can't have its
// container recreated, so restart falls back to a normal activation
// (which starts a fresh session). RestartSession is not called.
func TestRestartWorkerAgent_FallsBackToActivateWhenNoSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedOwnerPosition(t, st, ctx)
	mustCreateAIWorker(t, st, ctx, "w-bob", "p-root", "bob identity")

	restarter := &fakeRestarter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_bob", agentApp: "app_bob"}, disp)
	deps.WorkerRuntime = fakeWorkerRuntime{sessionID: ""} // no live session
	deps.SessionRestarter = restarter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/workers/w-bob/restart-agent", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	if restarter.calls != 0 {
		t.Errorf("RestartSession must NOT run without a live session; got %d", restarter.calls)
	}
	if disp.manualCalls != 1 {
		t.Errorf("DispatchManual must run as the fresh-start fallback; got %d", disp.manualCalls)
	}
}

// TestRestartWorkerAgent_404OnUnknownWorker pins the not-found path so a
// stale UI link returns a clean 404 before any restart side effects.
func TestRestartWorkerAgent_404OnUnknownWorker(t *testing.T) {
	deps, _, _ := newDeps(t)
	deps.WorkerRuntime = fakeWorkerRuntime{sessionID: "ses_ghost"}
	deps.SessionRestarter = &fakeRestarter{}
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/workers/w-ghost/restart-agent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}
