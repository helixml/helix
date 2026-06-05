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

	"github.com/google/jsonschema-go/jsonschema"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
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
	orgID        string
	workerID     orgchart.WorkerID
	envPath      string
	activationID activation.ID
}

func (f *fakeDispatcher) Dispatch(_ context.Context, _ streaming.Event) {
	f.mu.Lock()
	f.dispatchN++
	f.mu.Unlock()
}

func (f *fakeDispatcher) DispatchHire(_ context.Context, orgID string, workerID orgchart.WorkerID, envPath string, activationID activation.ID) {
	f.mu.Lock()
	f.hires = append(f.hires, dispatchHireCall{orgID: orgID, workerID: workerID, envPath: envPath, activationID: activationID})
	f.mu.Unlock()
}

func (f *fakeDispatcher) hireCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.hires)
}

func newHireTestEnv(t *testing.T) (Deps, *fakeDispatcher, string, orgchart.Worker) {
	t.Helper()
	st := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed enough to support hires off an existing position.
	ownerRole, err := orgchart.NewRole("r-owner", "# Owner", nil, nil, now, "org-test")
	if err != nil {
		t.Fatalf("new role: %v", err)
	}
	if err := st.Roles.Create(ctx, ownerRole); err != nil {
		t.Fatalf("create role: %v", err)
	}
	caller, _ := orgchart.NewHumanWorker("w-owner", "r-owner", nil, "", "org-test")
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
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-renee",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindHuman,
		IdentityContent: "# Renee",
	})
	out, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
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
	if _, err := deps.Store.Workers.Get(ctx, "org-test", "w-renee"); err != nil {
		t.Fatalf("worker row missing: %v", err)
	}
	// Environment row exists with expected envPath.
	env, err := deps.Store.Environments.Get(ctx, "org-test", "w-renee")
	if err != nil {
		t.Fatalf("environment row missing: %v", err)
	}
	wantPath := filepath.Join(envsDir, "w-renee")
	if env.Path != wantPath {
		t.Errorf("env path = %q, want %q", env.Path, wantPath)
	}
	// Human hires do NOT get an activation Stream.
	if _, err := deps.Store.Streams.Get(ctx, "org-test", streaming.StreamID("s-activations-w-renee")); err == nil {
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
// TestHireWorkerReturnsActivationID pins B5.8: a successful AI hire
// responds with both the new Worker ID and the hire-Activation ID,
// and the Activation row exists in the store at the moment the
// response is built — callers can immediately filter worker_log by
// the returned activation_id without racing the Spawner.
func TestHireWorkerReturnsActivationID(t *testing.T) {
	t.Parallel()
	deps, dispatcher, _, caller := newHireTestEnv(t)
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	raw, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var resp struct {
		ID           string `json:"id"`
		ActivationID string `json:"activation_id"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal hire response: %v", err)
	}
	if resp.ID != "w-alice" {
		t.Errorf("response.id = %q, want w-alice", resp.ID)
	}
	if resp.ActivationID == "" {
		t.Fatal("response.activation_id is empty; want a hire-Activation ID")
	}

	row, err := deps.Store.Activations.Get(context.Background(), "org-test", activation.ID(resp.ActivationID))
	if err != nil {
		t.Fatalf("activation %q not in store: %v", resp.ActivationID, err)
	}
	if row.WorkerID != "w-alice" {
		t.Errorf("row.WorkerID = %q, want w-alice", row.WorkerID)
	}
	if row.TranscriptStreamID != activation.StreamID("w-alice") {
		t.Errorf("row.TranscriptStreamID = %q, want %q", row.TranscriptStreamID, activation.StreamID("w-alice"))
	}
	if len(row.Triggers) != 1 || row.Triggers[0].Kind != activation.TriggerHire {
		t.Errorf("row.Triggers = %+v, want one hire trigger", row.Triggers)
	}
	if row.IsCompleted() {
		t.Error("row already completed at hire-response time; Spawner hasn't fired yet")
	}
	// DispatchHire was forwarded the same activation ID so the Spawner
	// reuses this row rather than creating a sibling.
	if n := dispatcher.hireCount(); n != 1 {
		t.Fatalf("DispatchHire calls = %d, want 1", n)
	}
	if got := dispatcher.hires[0].activationID; got != activation.ID(resp.ActivationID) {
		t.Errorf("DispatchHire activationID = %q, want %q", got, resp.ActivationID)
	}
}

func TestHireWorkerAICreatesActivationStreamAndDispatches(t *testing.T) {
	t.Parallel()
	deps, dispatcher, _, caller := newHireTestEnv(t)
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	ctx := context.Background()
	streamID := activation.StreamID("w-alice")
	if _, err := deps.Store.Streams.Get(ctx, "org-test", streamID); err != nil {
		t.Fatalf("activation stream missing: %v", err)
	}
	// Hiring worker's POSITION is subscribed (subs are position-anchored).
	if _, err := deps.Store.Subscriptions.Find(ctx, "org-test", "p-root", streamID); err != nil {
		t.Fatalf("hiring worker's position not subscribed to activation stream: %v", err)
	}
	// New worker's position is NOT subscribed (would loop the
	// dispatcher when the new worker publishes).
	newWorkerPos := orgchart.WorkerID("p-eng")
	if _, err := deps.Store.Subscriptions.Find(ctx, "org-test", newWorkerPos, streamID); err == nil {
		t.Fatalf("new worker's position must NOT be subscribed to its own activation stream")
	}
	// Dispatcher was called once.
	if n := dispatcher.hireCount(); n != 1 {
		t.Fatalf("DispatchHire calls = %d, want 1", n)
	}
}

// TestHireWorkerEnvDirCreated checks the on-disk Environment dir is
// created at the configured path.
func TestHireWorkerEnvDirCreated(t *testing.T) {
	t.Parallel()
	deps, _, envsDir, caller := newHireTestEnv(t)
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
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
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "",
	})
	_, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args})
	if err == nil {
		t.Fatal("expected error for empty identityContent")
	}
	if !strings.Contains(err.Error(), "identityContent") {
		t.Errorf("err = %v, want mention of identityContent", err)
	}
	// No worker row should have been written.
	if _, err := deps.Store.Workers.Get(context.Background(), "org-test", "w-alice"); err == nil {
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
	orgID    string
	workerID orgchart.WorkerID
	userID   string
}

func (h *captureHireHandler) OnHire(_ context.Context, orgID string, w orgchart.WorkerID, uid string) error {
	h.calls = append(h.calls, captureHireCall{orgID: orgID, workerID: w, userID: uid})
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
	tl := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args}); err != nil {
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
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
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
	tl := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	_, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args})
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
	tl := &HireWorker{deps: deps}

	ctx := runtimehelix.WithUserID(context.Background(), "u-phil")
	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(ctx, tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	state, err := runtimehelix.LoadState(context.Background(), deps.Store, "org-test", "w-alice")
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
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	state, err := runtimehelix.LoadState(context.Background(), deps.Store, "org-test", "w-alice")
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if state.HiringUserID != "" {
		t.Errorf("HiringUserID = %q, want empty", state.HiringUserID)
	}
}

// TestHireWorkerSchemaHasNoGrantsField pins the new contract: the
// hire_worker tool no longer accepts a `grants` field. A Worker's MCP
// surface is derived live from their Position's Role.Tools; per-Worker
// grants do not exist. Asserting on the JSON schema is the durable
// check — the schema is what the LLM sees.
func TestHireWorkerSchemaHasNoGrantsField(t *testing.T) {
	t.Parallel()
	tl := &HireWorker{}
	schema := tl.InputSchema()
	if schema == nil {
		t.Fatal("InputSchema() = nil")
	}
	if _, ok := schema.Properties["grants"]; ok {
		t.Errorf("hire_worker input schema still advertises `grants` property; "+
			"Role.Tools is now the live source of truth (got properties: %v)",
			propNames(schema.Properties))
	}
}

func propNames(m map[string]*jsonschema.Schema) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestHireWorkerInheritsCallerOrgID: the new Worker inherits its
// OrganizationID from the hiring caller's OrgID.
func TestHireWorkerInheritsCallerOrgID(t *testing.T) {
	t.Parallel()
	deps, _, _, caller := newHireTestEnv(t)
	tl := &HireWorker{deps: deps}

	args, _ := json.Marshal(hireWorkerArgs{
		ID:              "w-alice",
		RoleID:          "r-owner",
		Kind:            orgchart.WorkerKindAI,
		IdentityContent: "# Alice",
	})
	if _, err := tl.Invoke(context.Background(), tool.Invocation{Caller: caller, Args: args}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, err := deps.Store.Workers.Get(context.Background(), "org-test", "w-alice")
	if err != nil {
		t.Fatalf("Get hired worker: %v", err)
	}
	if got.OrganizationID() != "org-test" {
		t.Errorf("hired worker OrgID = %q, want org-test (inherited from caller)", got.OrganizationID())
	}
}
