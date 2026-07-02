package api_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
)

// fakeBotRuntime returns a fixed session id for the bot so the restart
// handler can decide between "reset the live session first" and "just
// activate" (first-time start).
type fakeBotRuntime struct {
	sessionID string
}

func (f fakeBotRuntime) State(_ context.Context, _ string, _ orgchart.BotID) (orgapi.BotRuntimeInfo, error) {
	return orgapi.BotRuntimeInfo{SessionID: f.sessionID}, nil
}

// fakeResetter records ResetSession calls so the test can assert the
// bot-page button fully tears the live session down before activating a
// brand-new one.
type fakeResetter struct {
	mu      sync.Mutex
	calls   int
	lastSID string
	err     error
}

func (f *fakeResetter) ResetSession(_ context.Context, _ string, _ orgchart.BotID, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastSID = sessionID
	return f.err
}

// TestRestartBotAgent_ResetsThenActivatesExistingSession pins the fix:
// when the bot has a live session, the bot-page "Restart agent session"
// button fully removes it via BotSessionResetter (stop desktop → delete
// session → clear pointer) and THEN enqueues an activation, so the bot
// lands on a brand-new session, desktop and thread — not the resumed old
// container.
func TestRestartBotAgent_ResetsThenActivatesExistingSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-alice", "# Alice")

	resetter := &fakeResetter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_alice", agentApp: "app_alice"}, disp)
	deps.BotRuntime = fakeBotRuntime{sessionID: "ses_alice"}
	deps.BotSessionResetter = resetter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-alice/restart-agent", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}

	if resetter.calls != 1 || resetter.lastSID != "ses_alice" {
		t.Errorf("ResetSession calls = %d lastSID = %q, want 1 / ses_alice", resetter.calls, resetter.lastSID)
	}
	if disp.manualCalls != 1 {
		t.Errorf("DispatchManual must run after the reset to start a fresh session; got %d", disp.manualCalls)
	}
}

// TestRestartBotAgent_ActivatesWithoutResetWhenNoSession pins the
// first-time path: a bot with no live session has nothing to tear down,
// so restart just activates. ResetSession is not called.
func TestRestartBotAgent_ActivatesWithoutResetWhenNoSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-bob", "# Bob")

	resetter := &fakeResetter{}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_bob", agentApp: "app_bob"}, disp)
	deps.BotRuntime = fakeBotRuntime{sessionID: ""} // no live session
	deps.BotSessionResetter = resetter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-bob/restart-agent", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	if resetter.calls != 0 {
		t.Errorf("ResetSession must NOT run without a live session; got %d", resetter.calls)
	}
	if disp.manualCalls != 1 {
		t.Errorf("DispatchManual must run to start a fresh session; got %d", disp.manualCalls)
	}
}

// TestRestartBotAgent_ResetFailureSurfaces pins that a failed teardown is
// reported (500) and does NOT fall through to an activation — we must not
// silently claim success while the old session lingers.
func TestRestartBotAgent_ResetFailureSurfaces(t *testing.T) {
	deps, st, _ := newDeps(t)
	ctx := context.Background()
	seedBot(t, st, ctx, "b-carol", "# Carol")

	resetter := &fakeResetter{err: errors.New("delete failed")}
	disp := &fakeDispatcher{}
	deps = wireActivate(deps, st, &fakeEnsurer{projectID: "prj_carol"}, disp)
	deps.BotRuntime = fakeBotRuntime{sessionID: "ses_carol"}
	deps.BotSessionResetter = resetter

	h := orgapi.Handler(deps)
	rec := do(t, h, "POST", "/bots/b-carol/restart-agent", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	if disp.manualCalls != 0 {
		t.Errorf("DispatchManual must NOT run when reset failed; got %d", disp.manualCalls)
	}
}

// TestRestartBotAgent_404OnUnknownBot pins the not-found path so a stale
// UI link returns a clean 404 before any restart side effects.
func TestRestartBotAgent_404OnUnknownBot(t *testing.T) {
	deps, _, _ := newDeps(t)
	deps.BotRuntime = fakeBotRuntime{sessionID: "ses_ghost"}
	deps.BotSessionResetter = &fakeResetter{}
	h := orgapi.Handler(deps)

	rec := do(t, h, "POST", "/bots/b-ghost/restart-agent", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
}
