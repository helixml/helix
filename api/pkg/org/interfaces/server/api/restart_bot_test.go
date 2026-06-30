package api_test

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// fakeBotRuntime returns a fixed session id for the bot so the restart
// handler can decide between "recreate existing container" and "fall
// back to a fresh activation".
type fakeBotRuntime struct {
	sessionID string
}

func (f fakeBotRuntime) State(_ context.Context, _ string, _ orgchart.BotID) (orgapi.BotRuntimeInfo, error) {
	return orgapi.BotRuntimeInfo{SessionID: f.sessionID}, nil
}

// fakeRestarter records RestartSession calls so the test can assert the
// bot-page button funnels into the shared backend restart primitive
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

// TestRestartBotAgent_RecreatesExistingSession pins the fix: when the bot
// has a live session, the bot-page "Restart agent session" button
// recreates that session's container via the SessionRestarter port (the
// shared backend primitive) — it does NOT enqueue a normal activation
// (which would SendMessage to the still-stuck container).
func TestRestartBotAgent_RecreatesExistingSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")

	restarter := &fakeRestarter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_alice"}, disp)
	deps.BotRuntime = fakeBotRuntime{sessionID: "ses_alice"}
	deps.SessionRestarter = restarter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-alice/restart-agent", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}

	var resp orgapi.BotActivateDTO
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

// TestRestartBotAgent_FallsBackToActivateWhenNoSession pins the
// first-time path: a bot with no live session can't have its container
// recreated, so restart falls back to a normal activation (which starts
// a fresh session). RestartSession is not called.
func TestRestartBotAgent_FallsBackToActivateWhenNoSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-bob", "# Bob")

	restarter := &fakeRestarter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_bob", agentApp: "app_bob"}, disp)
	deps.BotRuntime = fakeBotRuntime{sessionID: ""} // no live session
	deps.SessionRestarter = restarter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-bob/restart-agent", nil)
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

// TestRestartBotAgent_404OnUnknownBot pins the not-found path so a stale
// UI link returns a clean 404 before any restart side effects.
func TestRestartBotAgent_404OnUnknownBot(t *testing.T) {
	deps, _, _ := newDeps(t)
	deps.BotRuntime = fakeBotRuntime{sessionID: "ses_ghost"}
	deps.SessionRestarter = &fakeRestarter{}
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots/b-ghost/restart-agent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}
