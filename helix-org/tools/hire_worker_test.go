package tools

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/activation"
	"github.com/helixml/helix/api/pkg/org/role"
	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/stream"
	"github.com/helixml/helix/api/pkg/org/worker"
	"github.com/helixml/helix/helix-org/domain"
	"github.com/helixml/helix/helix-org/store/sqlite"
)

// fakeDispatcher records DispatchHire and Dispatch calls so the test
// can assert ordering of side-effects relative to hire_worker's
// downstream activation.
type fakeDispatcher struct {
	mu        sync.Mutex
	hires     []dispatchHireCall
	dispatchN int
}

type dispatchHireCall struct {
	workerID worker.ID
	envPath  string
}

func (f *fakeDispatcher) Dispatch(_ context.Context, _ domain.Event) {
	f.mu.Lock()
	f.dispatchN++
	f.mu.Unlock()
}

func (f *fakeDispatcher) DispatchHire(_ context.Context, workerID worker.ID, envPath string) {
	f.mu.Lock()
	f.hires = append(f.hires, dispatchHireCall{workerID: workerID, envPath: envPath})
	f.mu.Unlock()
}

func (f *fakeDispatcher) hireCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hires)
}

func newHireTestEnv(t *testing.T) (Deps, *fakeDispatcher, string, domain.Worker) {
	t.Helper()
	st, err := sqlite.Open(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed enough to support hires off an existing position.
	ownerRole, err := role.New("r-owner", "# Owner", nil, nil, now)
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, ownerRole); err != nil {
		t.Fatalf("create role: %v", err)
	}
	rootPos, _ := domain.NewPosition("p-root", "r-owner", nil)
	if err := st.Positions.Create(ctx, rootPos); err != nil {
		t.Fatalf("create position: %v", err)
	}
	caller, _ := domain.NewHumanWorker("w-owner", "p-root", "")
	if err := st.Workers.Create(ctx, caller); err != nil {
		t.Fatalf("create owner worker: %v", err)
	}

	envsDir := t.TempDir()
	dispatcher := &fakeDispatcher{}
	deps := DefaultDeps(st)
	deps.EnvsDir = envsDir
	deps.Dispatcher = dispatcher
	deps.Now = func() time.Time { return now }
	// Deterministic IDs make assertions on stream + grant IDs feasible.
	var counter int
	deps.NewID = func() string {
		counter++
		return "id" + strings.Repeat("0", 1) + string(rune('a'+counter-1))
	}
	return deps, dispatcher, envsDir, caller
}

// TestHireWorkerHumanCreatesRowsAndSkipsActivation verifies that a
// human hire writes Worker + Environment rows, but does NOT create an
// activation Stream and does NOT call DispatchHire. This is today's
// behaviour we MUST preserve through the B4 refactor.
func TestHireWorkerHumanCreatesRowsAndSkipsActivation(t *testing.T) {
	t.Parallel()
	deps, dispatcher, envsDir, caller := newHireTestEnv(t)
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-renee",
		PositionID:      "p-root",
		Kind:            worker.KindHuman,
		IdentityContent: "# Renee",
	})
	out, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	var got struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ID != "w-renee" {
		t.Fatalf("returned id = %q, want w-renee", got.ID)
	}

	ctx := context.Background()
	// Worker row exists.
	if _, err := deps.Store.Workers.Get(ctx, "w-renee"); err != nil {
		t.Fatalf("worker row missing: %v", err)
	}
	// Environment row exists with expected envPath.
	env, err := deps.Store.Environments.Get(ctx, "w-renee")
	if err != nil {
		t.Fatalf("environment row missing: %v", err)
	}
	wantPath := filepath.Join(envsDir, "w-renee")
	if env.Path != wantPath {
		t.Errorf("env path = %q, want %q", env.Path, wantPath)
	}
	// Human hires do NOT get an activation Stream.
	if _, err := deps.Store.Streams.Get(ctx, stream.ID("s-activations-w-renee")); err == nil {
		t.Fatalf("human hire must NOT create activation stream")
	}
	// Human hires do NOT trigger the dispatcher.
	if n := dispatcher.hireCount(); n != 0 {
		t.Fatalf("DispatchHire called %d times for human hire (want 0)", n)
	}
}

// TestHireWorkerAICreatesActivationStreamAndDispatches verifies an AI
// hire ALSO creates the activation Stream, subscribes the hiring
// Worker to it, and calls DispatchHire. This is the full AI-hire
// recipe.
func TestHireWorkerAICreatesActivationStreamAndDispatches(t *testing.T) {
	t.Parallel()
	deps, dispatcher, _, caller := newHireTestEnv(t)
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	ctx := context.Background()
	streamID := activation.StreamID("w-alice")
	if _, err := deps.Store.Streams.Get(ctx, streamID); err != nil {
		t.Fatalf("activation stream missing: %v", err)
	}
	// Hiring worker (caller) is subscribed.
	if _, err := deps.Store.Subscriptions.Find(ctx, "w-owner", streamID); err != nil {
		t.Fatalf("hiring worker not subscribed to activation stream: %v", err)
	}
	// New worker is NOT subscribed (would loop the dispatcher).
	if _, err := deps.Store.Subscriptions.Find(ctx, "w-alice", streamID); err == nil {
		t.Fatalf("new worker must NOT be subscribed to its own activation stream")
	}
	// Dispatcher was called once.
	if n := dispatcher.hireCount(); n != 1 {
		t.Fatalf("DispatchHire calls = %d, want 1", n)
	}
}

// TestHireWorkerGrantsBeforeDispatch checks that all bundled grants
// land in the store BEFORE the dispatcher fires. This is load-bearing
// — an AI Worker that activates before its grants land will 403 on its
// first tool call.
func TestHireWorkerGrantsBeforeDispatch(t *testing.T) {
	t.Parallel()
	deps, dispatcher, _, caller := newHireTestEnv(t)

	// Wrap the dispatcher so we can read the grant count AT the moment
	// DispatchHire fires.
	var grantsAtDispatch int
	captured := dispatcherCapturingGrants{
		inner: dispatcher,
		onHire: func() {
			grants, _ := deps.Store.Grants.ListByWorker(context.Background(), "w-alice")
			grantsAtDispatch = len(grants)
		},
	}
	deps.Dispatcher = &captured
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
		Grants: []hireWorkerGrant{
			{ToolName: "publish"},
			{ToolName: "subscribe"},
		},
	})
	if _, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if grantsAtDispatch != 2 {
		t.Fatalf("grants present at DispatchHire time = %d, want 2", grantsAtDispatch)
	}
}

type dispatcherCapturingGrants struct {
	inner  *fakeDispatcher
	onHire func()
}

func (d *dispatcherCapturingGrants) Dispatch(ctx context.Context, e domain.Event) {
	d.inner.Dispatch(ctx, e)
}

func (d *dispatcherCapturingGrants) DispatchHire(ctx context.Context, w worker.ID, envPath string) {
	if d.onHire != nil {
		d.onHire()
	}
	d.inner.DispatchHire(ctx, w, envPath)
}

// TestHireWorkerEnvDirCreated checks the on-disk Environment dir is
// created at the configured path.
func TestHireWorkerEnvDirCreated(t *testing.T) {
	t.Parallel()
	deps, _, envsDir, caller := newHireTestEnv(t)
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	wantPath := filepath.Join(envsDir, "w-alice")
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat %s: %v", wantPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("%s is not a directory", wantPath)
	}
}

// TestHireWorkerMissingIdentityRejected checks the hire fails fast
// before any DB row is written when identityContent is empty.
func TestHireWorkerMissingIdentityRejected(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "",
	})
	_, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args})
	if err == nil {
		t.Fatal("expected error for empty identityContent")
	}
	if !strings.Contains(err.Error(), "identityContent") {
		t.Errorf("err = %v, want mention of identityContent", err)
	}
	// No worker row should have been written.
	if _, err := deps.Store.Workers.Get(context.Background(), "w-alice"); err == nil {
		t.Fatal("worker row created despite identity rejection")
	}
}

// captureHireHandler records OnHire calls. Used to assert hire_worker
// routes through runtime.HireHook rather than calling
// SaveHiringUser directly.
type captureHireHandler struct {
	calls   []captureHireCall
	failErr error
}

type captureHireCall struct {
	workerID worker.ID
	userID   string
}

func (h *captureHireHandler) OnHire(_ context.Context, w worker.ID, uid string) error {
	h.calls = append(h.calls, captureHireCall{workerID: w, userID: uid})
	return h.failErr
}

// TestHireWorkerInvokesHireHandlerWithUserID verifies the new B4 flow:
// hire_worker routes the hiring-user side-effect through the
// HireHook port, not via a direct SaveHiringUser call.
func TestHireWorkerInvokesHireHandlerWithUserID(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	hook := &captureHireHandler{}
	deps.HireHook = hook
	tool := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(ctx, domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(hook.calls) != 1 {
		t.Fatalf("OnHire calls = %d, want 1", len(hook.calls))
	}
	if hook.calls[0].workerID != "w-alice" || hook.calls[0].userID != "u-phil" {
		t.Errorf("OnHire(%q, %q), want (w-alice, u-phil)", hook.calls[0].workerID, hook.calls[0].userID)
	}
}

// TestHireWorkerSkipsHireHandlerWithoutUserID confirms the no-op
// path: without a userID in context, the hook is NOT invoked.
// Matches the prior contract that unauthenticated hires don't write
// any hiring-user state.
func TestHireWorkerSkipsHireHandlerWithoutUserID(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	hook := &captureHireHandler{}
	deps.HireHook = hook
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(hook.calls) != 0 {
		t.Errorf("OnHire called %d times without userID in ctx (want 0)", len(hook.calls))
	}
}

// TestHireWorkerHireHandlerErrorIsFatal verifies the error contract:
// an OnHire failure aborts the hire with a wrapped error. Matches
// the existing fatal behaviour at hire_worker.go (the doc comment
// said non-fatal but the code returned the error).
func TestHireWorkerHireHandlerErrorIsFatal(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	hook := &captureHireHandler{failErr: errors.New("boom")}
	deps.HireHook = hook
	tool := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	_, err := tool.Invoke(ctx, domain.Invocation{Caller: caller, Args: args})
	if err == nil {
		t.Fatal("expected error when hook fails")
	}
	if !strings.Contains(err.Error(), "hire handler") {
		t.Errorf("err = %v, want mention of 'hire handler'", err)
	}
}

// TestHireWorkerPersistsHiringUserFromContext verifies that when the
// request context carries a userID, the runtime state for the new
// Worker has it persisted. This pins the existing
// `agenthelix.SaveHiringUser(...)` side-effect, which the B4 refactor
// must preserve via the HireHook port.
func TestHireWorkerPersistsHiringUserFromContext(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	// Wire the real Hire so the persistence side-effect runs
	// end-to-end. NoopHireHook is the default and would skip the
	// SaveHiringUser side-effect.
	deps.HireHook = &runtimehelix.Hire{Store: deps.Store}
	tool := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(ctx, domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	state, err := runtimehelix.LoadState(context.Background(), deps.Store, "w-alice")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.HiringUserID != "u-phil" {
		t.Errorf("HiringUserID = %q, want u-phil", state.HiringUserID)
	}
}

// TestHireWorkerWithoutUserIDDoesNotPersist confirms the no-op path:
// when the context has no userID, the Worker's runtime state has no
// HiringUserID stored. Tests the contract that
// `agenthelix.SaveHiringUser` (and the future HireHook) must
// preserve.
func TestHireWorkerWithoutUserIDDoesNotPersist(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	tool := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		PositionID:      "p-root",
		Kind:            worker.KindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tool.Invoke(context.Background(), domain.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	state, err := runtimehelix.LoadState(context.Background(), deps.Store, "w-alice")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.HiringUserID != "" {
		t.Errorf("HiringUserID = %q, want empty", state.HiringUserID)
	}
}
