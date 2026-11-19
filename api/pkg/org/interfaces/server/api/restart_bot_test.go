package api_test

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/activations"
	"github.com/helixml/helix/api/pkg/org/domain/audit"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	"github.com/helixml/helix/api/pkg/types"
)

// fakeBotRuntime returns a fixed session id for the bot.
type fakeBotRuntime struct {
	sessionID string
}

func (f fakeBotRuntime) State(_ context.Context, _ string, _ orgchart.NodeID) (orgapi.BotRuntimeInfo, error) {
	return orgapi.BotRuntimeInfo{SessionID: f.sessionID}, nil
}

type fakeResetter struct {
	mu      sync.Mutex
	calls   int
	lastSID string
	err     error
}

func (f *fakeResetter) ResetSession(_ context.Context, _ string, _ orgchart.NodeID, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.lastSID = sessionID
	return f.err
}

type fakeRuntimeRestarter struct {
	calls int
	err   error
}

func (f *fakeRuntimeRestarter) RestartRuntime(context.Context, string, orgchart.NodeID) error {
	f.calls++
	return f.err
}

type fakeCanceller struct{ calls int }

func (f *fakeCanceller) CancelOutstanding(context.Context, string, orgchart.NodeID) error {
	f.calls++
	return nil
}

type fakeHomeEraser struct {
	inSession bool
	retired   int
}

func (f *fakeHomeEraser) HomeMayBeInSession(context.Context, string, orgchart.NodeID) (bool, error) {
	return f.inSession, nil
}

func (f *fakeHomeEraser) RetireHome(context.Context, string, orgchart.NodeID) (activations.HomeErasure, error) {
	f.retired++
	return activations.HomeErasure{Existed: true, Generation: "bh_1", RetiredPath: "/data/workspaces/org-bots-retired/org-test/b/bh_1-1"}, nil
}

type fakeAuditRecorder struct{ entries []audit.Entry }

func (f *fakeAuditRecorder) Record(_ context.Context, e audit.Entry) error {
	f.entries = append(f.entries, e)
	return nil
}

type lifecycleFakes struct {
	resetter  *fakeResetter
	restarter *fakeRuntimeRestarter
	canceller *fakeCanceller
	homes     *fakeHomeEraser
	audit     *fakeAuditRecorder
	disp      *fakeDispatcher
}

func wireLifecycle(t *testing.T, deps orgapi.Deps, st interface{}, sessionID string) (orgapi.Deps, *lifecycleFakes) {
	t.Helper()
	f := &lifecycleFakes{
		resetter:  &fakeResetter{},
		restarter: &fakeRuntimeRestarter{},
		canceller: &fakeCanceller{},
		homes:     &fakeHomeEraser{},
		audit:     &fakeAuditRecorder{},
		disp:      &fakeDispatcher{},
	}
	deps.BotRuntime = fakeBotRuntime{sessionID: sessionID}
	deps.Activations = activations.New(activations.Deps{
		NewID:      func() string { return "act-1" },
		Ensurer:    &fakeEnsurer{projectID: "prj_1", agentApp: "app_1"},
		Dispatcher: f.disp,
		Sessions:   botRuntimeSessionAdapter{rt: deps.BotRuntime},
		Resetter:   f.resetter,
		Canceller:  f.canceller,
		Restarter:  f.restarter,
		Homes:      f.homes,
		Audit:      f.audit,
	})
	return deps, f
}

// Restart is the ordinary, non-destructive operation: same session, no
// queue purge, no session delete, no home erase.
func TestRestartBot_RecreatesRuntimeWithoutDestroyingState(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-alice", "# Alice")
	deps, f := wireLifecycle(t, deps, st, "ses_alice")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-alice/runtime-restart", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.BotRestartDTO
	decode(t, rec, &got)
	if got.Mode != "runtime" || got.SessionID != "ses_alice" || len(got.Preserves) == 0 {
		t.Fatalf("response = %+v", got)
	}
	if f.restarter.calls != 1 {
		t.Fatalf("runtime restarts = %d, want 1", f.restarter.calls)
	}
	if f.resetter.calls != 0 || f.canceller.calls != 0 || f.homes.retired != 0 || f.disp.manualCalls != 0 {
		t.Fatalf("restart was destructive: reset=%d cancel=%d retired=%d dispatched=%d",
			f.resetter.calls, f.canceller.calls, f.homes.retired, f.disp.manualCalls)
	}
}

func TestRestartBot_FirstStartActivates(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-bob", "# Bob")
	deps, f := wireLifecycle(t, deps, st, "")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-bob/runtime-restart", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.BotRestartDTO
	decode(t, rec, &got)
	if got.Mode != "first_start" || got.ProjectID != "prj_1" {
		t.Fatalf("response = %+v", got)
	}
	if f.restarter.calls != 0 || f.resetter.calls != 0 || f.disp.manualCalls != 1 {
		t.Fatalf("restarter=%d reset=%d dispatch=%d", f.restarter.calls, f.resetter.calls, f.disp.manualCalls)
	}
}

func TestRestartBot_FailureSurfacesWithoutReset(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-carol", "# Carol")
	deps, f := wireLifecycle(t, deps, st, "ses_carol")
	f.restarter.err = errors.New("hydra down")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-carol/runtime-restart", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500; body=%s", rec.Code, rec.Body)
	}
	if f.resetter.calls != 0 || f.canceller.calls != 0 || f.disp.manualCalls != 0 {
		t.Fatal("a failed restart must not fall back to a reset")
	}
}

func TestRestartBot_404OnUnknownBot(t *testing.T) {
	deps, st, _ := newDeps(t)
	deps, f := wireLifecycle(t, deps, st, "ses_ghost")
	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-ghost/runtime-restart", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body=%s", rec.Code, rec.Body)
	}
	if f.restarter.calls != 0 {
		t.Fatal("restarted an unknown bot")
	}
}

func TestResetBot_SessionScope(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-dave", "# Dave")
	deps, f := wireLifecycle(t, deps, st, "ses_dave")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-dave/reset", orgapi.BotResetRequestDTO{Scope: "session", Confirm: "b-dave"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.BotResetDTO
	decode(t, rec, &got)
	if got.Scope != "session" || got.PreviousSessionID != "ses_dave" || got.Home != nil || len(got.Erases) == 0 || len(got.Keeps) == 0 {
		t.Fatalf("response = %+v", got)
	}
	if f.canceller.calls != 1 || f.resetter.lastSID != "ses_dave" || f.homes.retired != 0 || f.disp.manualCalls != 1 {
		t.Fatalf("cancel=%d reset=%q retired=%d dispatch=%d", f.canceller.calls, f.resetter.lastSID, f.homes.retired, f.disp.manualCalls)
	}
	if len(f.audit.entries) != 2 || f.audit.entries[1].Status != audit.StatusSucceeded || f.audit.entries[1].ActorType != audit.ActorUser {
		t.Fatalf("audit = %+v", f.audit.entries)
	}
}

func TestResetBot_SessionAndHomeScope(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-erin", "# Erin")
	deps, f := wireLifecycle(t, deps, st, "ses_erin")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-erin/reset", orgapi.BotResetRequestDTO{Scope: "session_and_home", Confirm: "b-erin"})
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	var got orgapi.BotResetDTO
	decode(t, rec, &got)
	if got.Home == nil || got.Home.RetiredPath == "" || f.homes.retired != 1 {
		t.Fatalf("home not retired: %+v", got)
	}
}

func TestResetBot_RejectsMissingScopeOrConfirmation(t *testing.T) {
	for _, body := range []any{
		orgapi.BotResetRequestDTO{Scope: "session"},
		orgapi.BotResetRequestDTO{Scope: "session", Confirm: "b-other"},
		orgapi.BotResetRequestDTO{Scope: "all", Confirm: "b-frank"},
		nil,
	} {
		deps, st, _ := newDeps(t)
		seedBot(t, st, context.Background(), "b-frank", "# Frank")
		deps, f := wireLifecycle(t, deps, st, "ses_frank")
		rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-frank/reset", body)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%+v: status = %d, want 400; body=%s", body, rec.Code, rec.Body)
		}
		if f.canceller.calls != 0 || f.resetter.calls != 0 || f.homes.retired != 0 {
			t.Fatalf("%+v: invalid reset had side effects", body)
		}
	}
}

func TestResetBot_RefusesSessionScopeWhileHomeLivesInSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-gail", "# Gail")
	deps, f := wireLifecycle(t, deps, st, "ses_gail")
	f.homes.inSession = true

	for _, scope := range []string{"session", "session_and_home"} {
		rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-gail/reset", orgapi.BotResetRequestDTO{Scope: scope, Confirm: "b-gail"})
		if rec.Code != http.StatusConflict {
			t.Fatalf("%s: status = %d, want 409; body=%s", scope, rec.Code, rec.Body)
		}
	}
	if f.canceller.calls != 0 || f.resetter.calls != 0 || f.homes.retired != 0 || f.disp.manualCalls != 0 {
		t.Fatal("refused reset had side effects")
	}
}

// A member without edit rights on the bot cannot reset it, and nothing is
// cancelled, deleted or erased.
func TestResetBot_RefusesMemberWithoutEditRights(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-hank", "# Hank")
	deps, f := wireLifecycle(t, deps, st, "ses_hank")

	for _, path := range []string{"/bots/b-hank/reset", "/bots/b-hank/restart", "/bots/b-hank/runtime-restart"} {
		rec := doAsRole(t, orgapi.Handler(deps), "POST", path, orgapi.BotResetRequestDTO{Scope: "session_and_home", Confirm: "b-hank"}, types.OrganizationRoleMember, false)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: status = %d, want 403; body=%s", path, rec.Code, rec.Body)
		}
	}
	if f.canceller.calls != 0 || f.resetter.calls != 0 || f.homes.retired != 0 || f.restarter.calls != 0 || len(f.audit.entries) != 0 {
		t.Fatal("unauthorized request had side effects")
	}
}

// Another tenant's bot id resolves to 404 in this org and nothing happens.
func TestResetBot_CrossTenantBotIsNotFound(t *testing.T) {
	deps, st, _ := newDeps(t)
	other, err := orgchart.NewNode("b-other", "# Other", nil, time.Now().UTC(), "org-other")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	deps, f := wireLifecycle(t, deps, st, "ses_other")

	for _, path := range []string{"/bots/b-other/reset", "/bots/b-other/restart", "/bots/b-other/runtime-restart"} {
		rec := do(t, orgapi.Handler(deps), "POST", path, orgapi.BotResetRequestDTO{Scope: "session_and_home", Confirm: "b-other"})
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404; body=%s", path, rec.Code, rec.Body)
		}
	}
	if f.canceller.calls != 0 || f.resetter.calls != 0 || f.homes.retired != 0 || f.restarter.calls != 0 {
		t.Fatal("cross-tenant request had side effects")
	}
	if _, err := st.Nodes.Get(context.Background(), "org-other", "b-other"); err != nil {
		t.Fatalf("other tenant's bot changed: %v", err)
	}
}

// The legacy endpoint keeps its documented contract (fresh session, queue
// purged) but runs as an audited session-scope reset that keeps the home.
func TestLegacyRestartBot_KeepsFreshSessionContractWithoutErasingHome(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-ivy", "# Ivy")
	deps, f := wireLifecycle(t, deps, st, "ses_ivy")

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-ivy/restart", nil)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", rec.Code, rec.Body)
	}
	if rec.Header().Get("Deprecation") != "true" {
		t.Fatal("legacy restart must announce its deprecation")
	}
	var got orgapi.BotActivateDTO
	decode(t, rec, &got)
	if got.ProjectID != "prj_1" {
		t.Fatalf("response = %+v", got)
	}
	if f.canceller.calls != 1 || f.resetter.lastSID != "ses_ivy" || f.disp.manualCalls != 1 {
		t.Fatalf("legacy contract broken: cancel=%d reset=%q dispatch=%d", f.canceller.calls, f.resetter.lastSID, f.disp.manualCalls)
	}
	if f.homes.retired != 0 || f.restarter.calls != 0 {
		t.Fatal("legacy restart must not erase the bot home")
	}
	if len(f.audit.entries) != 2 || f.audit.entries[1].Action != "reset_bot" {
		t.Fatalf("legacy restart not audited: %+v", f.audit.entries)
	}
}

func TestLegacyRestartBot_RefusesWhileHomeLivesInSession(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-jo", "# Jo")
	deps, f := wireLifecycle(t, deps, st, "ses_jo")
	f.homes.inSession = true

	rec := do(t, orgapi.Handler(deps), "POST", "/bots/b-jo/restart", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body=%s", rec.Code, rec.Body)
	}
	if f.resetter.calls != 0 || f.canceller.calls != 0 {
		t.Fatal("refused legacy restart had side effects")
	}
}

func TestGetBotResetPlan(t *testing.T) {
	deps, st, _ := newDeps(t)
	seedBot(t, st, context.Background(), "b-kim", "# Kim")
	deps, f := wireLifecycle(t, deps, st, "ses_kim")
	f.homes.inSession = true

	rec := do(t, orgapi.Handler(deps), "GET", "/bots/b-kim/reset", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body)
	}
	var plan orgapi.BotResetPlanDTO
	decode(t, rec, &plan)
	if len(plan.Scopes) != 2 || plan.Scopes[0].Scope != "session" || plan.Scopes[1].Scope != "session_and_home" {
		t.Fatalf("plan = %+v", plan)
	}
	for _, sc := range plan.Scopes {
		if sc.Available || sc.UnavailableReason == "" {
			t.Fatalf("scope %s must be unavailable while the home lives in the session: %+v", sc.Scope, plan)
		}
	}
	for _, sc := range plan.Scopes {
		if len(sc.Erases) == 0 || len(sc.Keeps) == 0 {
			t.Fatalf("scope %s has no declaration", sc.Scope)
		}
	}
	if f.canceller.calls != 0 || f.resetter.calls != 0 || len(f.audit.entries) != 0 {
		t.Fatal("reading the plan had side effects")
	}

	rec = doAsRole(t, orgapi.Handler(deps), "GET", "/bots/b-kim/reset", nil, types.OrganizationRoleMember, false)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("member: status = %d, want 403", rec.Code)
	}
	rec = do(t, orgapi.Handler(deps), "GET", "/bots/b-nobody/reset", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown bot: status = %d, want 404", rec.Code)
	}
}
