package helix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	orggorm "github.com/helixml/helix/api/pkg/org/infrastructure/persistence/gorm"
	"github.com/helixml/helix/api/pkg/pubsub"
)

// fakeHelixClient is a deterministic stand-in for Client.
type fakeHelixClient struct {
	mu                 sync.Mutex
	startCalls         int32
	sendCalls          int32
	outputCalls        int32
	subscribeCalls     int32
	startSessionID     string
	sessionOwner       string // returned by SessionOwner; the transcript bridge subscribes to this owner's pubsub topic
	exploratorySession string // returned by ExploratorySession; the mirror polls this to track the worker's current session
	startErr           error
	sendErr            error
	outputs            []types.SessionOutputResponse
	updatesFactory     func() <-chan types.WebsocketEvent
	lastStartParams    StartSessionParams
	lastSendSID        string
	lastSendBody       string
	clearCalls         int32
	lastClearSID       string
	// clearedBeforeSend records whether ClearSession ran before the
	// first SendMessage, so tests can assert the activation clears the
	// prior conversation ahead of dispatching the new prompt.
	clearedBeforeSend bool
	clearErr          error
	// startBlock, when non-nil, blocks StartSession until the channel
	// closes or the caller's context is done — lets tests verify that
	// the spawner's SessionStartupTimeout actually bounds session
	// creation. nil means StartSession returns immediately.
	startBlock <-chan struct{}
}

func (f *fakeHelixClient) StartSession(ctx context.Context, params StartSessionParams) (string, error) {
	atomic.AddInt32(&f.startCalls, 1)
	f.mu.Lock()
	f.lastStartParams = params
	block := f.startBlock
	f.mu.Unlock()
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.startErr != nil {
		return "", f.startErr
	}
	return f.startSessionID, nil
}

func (f *fakeHelixClient) SendMessage(_ context.Context, sessionID, prompt string) error {
	atomic.AddInt32(&f.sendCalls, 1)
	f.mu.Lock()
	f.lastSendSID = sessionID
	f.lastSendBody = prompt
	f.mu.Unlock()
	return f.sendErr
}

func (f *fakeHelixClient) ClearSession(_ context.Context, sessionID string) error {
	atomic.AddInt32(&f.clearCalls, 1)
	f.mu.Lock()
	f.lastClearSID = sessionID
	// The clear must precede the prompt dispatch — if no SendMessage has
	// run yet, this clear happened first.
	if atomic.LoadInt32(&f.sendCalls) == 0 {
		f.clearedBeforeSend = true
	}
	clearErr := f.clearErr
	f.mu.Unlock()
	return clearErr
}

func (f *fakeHelixClient) GetOutput(_ context.Context, _ string) (types.SessionOutputResponse, error) {
	i := int(atomic.AddInt32(&f.outputCalls, 1)) - 1
	f.mu.Lock()
	defer f.mu.Unlock()
	if i >= len(f.outputs) {
		return f.outputs[len(f.outputs)-1], nil
	}
	return f.outputs[i], nil
}

func (f *fakeHelixClient) StopExternalAgent(_ context.Context, _ string) error { return nil }
func (f *fakeHelixClient) SessionOwner(_ context.Context, _ string) (string, error) {
	return f.sessionOwner, nil
}

// exploratorySession + setExploratory back the Mirror's session
// resolver. The mirror polls this to track the worker as its session
// changes; tests flip it to simulate session churn.
func (f *fakeHelixClient) setExploratory(sid string) {
	f.mu.Lock()
	f.exploratorySession = sid
	f.mu.Unlock()
}
func (f *fakeHelixClient) ExploratorySession(_ context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.exploratorySession, nil
}
func (f *fakeHelixClient) ServerStatus(_ context.Context) (ServerStatus, error) {
	return ServerStatus{MaxConcurrentDesktops: 0, ActiveConcurrentDesktops: 0}, nil
}

func newHelixTestStore(t *testing.T) (*store.Store, orgchart.BotID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	bot, _ := orgchart.NewBot("w-eng", "# Role: Engineer", nil, nil, time.Now().UTC(), "org-test")
	if err := s.Bots.Create(ctx, bot); err != nil {
		t.Fatalf("bot: %v", err)
	}
	return s, bot.ID
}

func newHelixCfg(t *testing.T, fc SpawnerClient, s *store.Store) SpawnerConfig {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return SpawnerConfig{
		Client:                 fc,
		ProjectService:         newFakeProjectService(),
		Workspace:              NewWorkspace(newFakeGitForProject(), s, "helix-specs", "helix-org", "ho@example.com"),
		PubSub:                 newFakePubSub(),
		Snapshotter:            NoopSessionPreamble{},
		HelixOrgURL:            "http://helix-org:8081",
		Provider:               "openai",
		Model:                  "gpt-4o-mini",
		SessionStartupTimeout:  2 * time.Second,
		ActivationRunawayGuard: 2 * time.Second,
		MaxInflight:            2,
		PollInitial:            time.Millisecond,
		PollMax:                5 * time.Millisecond,
		Logger:                 logger,
		Store:                  s,
		Now:                    func() time.Time { return time.Now().UTC() },
		NewID:                  func() string { return "id" },
	}
}

func TestSpawnerStartsFreshAndPersistsSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Errorf("StartChat calls: %d", got)
	}
	// First activation has no prior session, so there is nothing to clear.
	if got := atomic.LoadInt32(&fc.clearCalls); got != 0 {
		t.Errorf("ClearSession called %d times on a first activation; a fresh StartSession has no prior context to wipe", got)
	}
	state, err := LoadState(context.Background(), s, "org-test", wid)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.SessionID != "ses_new" {
		t.Errorf("session pointer = %q", state.SessionID)
	}
	// The Worker should have its per-project IDs persisted from the
	// fake's ApplyProject response.
	if state.ProjectID != "prj_test" || state.AgentAppID != "app_test" {
		t.Errorf("project IDs not persisted: project=%q agent_app=%q", state.ProjectID, state.AgentAppID)
	}
	// StartSession must point at the per-Worker project, not at any
	// global one.
	if fc.lastStartParams.ProjectID != "prj_test" {
		t.Errorf("StartSession ProjectID = %q (want prj_test)", fc.lastStartParams.ProjectID)
	}
	if fc.lastStartParams.AppID != "app_test" {
		t.Errorf("StartSession AppID = %q (want app_test)", fc.lastStartParams.AppID)
	}
}

// TestSpawnerAttachesHelixOrgMCPEveryActivation pins the bug-fix
// invariant: the helix-org MCP is re-attached to the Worker's agent
// app on every activation, after ensureProject runs. helix's project-
// apply path wholesale-replaces Config.Helix on update, so without
// this re-attach the MCP is gone from the second activation onward
// and the desktop runtime boots without the helix-org tools — the
// regression behind /workers/<id>/mcp not appearing in Zed.
func TestSpawnerAttachesHelixOrgMCPEveryActivation(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.MCPAuthBearer = "k_service"
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent}}); err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	svc := cfg.ProjectService.(*fakeProjectService)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	// Two activations should mean two UpdateAppConfig calls for the
	// MCP attach (one per activation, after ensureProject). The
	// helix-side apply path clobbers the MCP list each time so this
	// must NOT be debounced.
	if svc.updateAppCalls != 2 {
		t.Errorf("UpdateAppConfig calls = %d, want 2 (one MCP re-attach per activation)", svc.updateAppCalls)
	}
	mcp := findMCP(svc.updateAppLastCfg, HelixOrgMCPName)
	if mcp == nil {
		t.Fatalf("last UpdateApp missing helix MCP entry: %+v", svc.updateAppLastCfg)
	}
	if !strings.HasSuffix(mcp.URL, "/workers/w-eng/mcp") {
		t.Errorf("MCP URL = %q, want /workers/w-eng/mcp suffix", mcp.URL)
	}
	if mcp.Headers["Authorization"] != "Bearer k_service" {
		t.Errorf("MCP fallback bearer not applied; Authorization = %q", mcp.Headers["Authorization"])
	}
}

// TestBridgeRendersEntryPatchEvents verifies that the bridge's
// EntryTopic callback produces the same line shapes the claude
// bridge emits — assistant text, tool_use, tool_result.
func TestBridgeRendersEntryPatchEvents(t *testing.T) {
	t.Parallel()
	var got []string
	b := newBridge(func(s string) { got = append(got, s) })
	b.topic.Apply(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi", PatchOffset: 0},
	}})
	b.topic.Apply(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 1, MessageID: "t1", Type: "tool_call", Patch: `{"x":1}`, ToolName: "publish", ToolStatus: "Completed"},
	}})
	b.topic.Flush()
	if len(got) < 3 {
		t.Fatalf("expected ≥3 events, got %d: %v", len(got), got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "assistant: hi") {
		t.Errorf("missing assistant: %v", got)
	}
	if !strings.Contains(joined, "tool_use publish: {\"x\":1}") {
		t.Errorf("missing tool_use: %v", got)
	}
	if !strings.Contains(joined, "tool_result: ") {
		t.Errorf("missing tool_result: %v", got)
	}
}

// TestSpawnerFollowUpResumesPersistedSession verifies that a Worker
// with a persisted Helix session ID resumes that session rather than
// opening a fresh one — the activation prompt is sent against the
// existing session_id so the warm desktop container is reused. This
// is what makes follow-up activations land in seconds against a warm
// desktop. Resume goes through StartChatWithStatus with SessionID
// set (EnsureAndSend's resume path).
func TestSpawnerFollowUpResumesPersistedSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	// Pre-seed an existing project + session for this worker.
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_existing"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_existing",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// A follow-up with a persisted session sends via SendMessage to the
	// existing session — no fresh StartSession, no churn. The session
	// pointer must remain unchanged.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastSendSID != "ses_existing" {
		t.Errorf("SendMessage sessionID = %q (want ses_existing) — follow-up must target persisted session", fc.lastSendSID)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Errorf("StartSession called %d times; a follow-up must reuse the session, not create a fresh one", got)
	}
	// The persisted session must be cleared exactly once, and before the
	// prompt is dispatched, so the worker turn starts on a fresh context
	// window rather than re-using the prior (potentially huge) transcript.
	if got := atomic.LoadInt32(&fc.clearCalls); got != 1 {
		t.Errorf("ClearSession called %d times; a follow-up must clear the session exactly once before re-activation", got)
	}
	if fc.lastClearSID != "ses_existing" {
		t.Errorf("ClearSession sessionID = %q (want ses_existing) — must clear the persisted session", fc.lastClearSID)
	}
	if !fc.clearedBeforeSend {
		t.Error("ClearSession must run BEFORE SendMessage — otherwise the new prompt lands on the old context window")
	}
	state, _ := LoadState(context.Background(), s, "org-test", wid)
	if state.SessionID != "ses_existing" {
		t.Errorf("session pointer changed to %q; follow-up must NOT open a fresh session", state.SessionID)
	}
}

// TestSpawnerPreservesContextWhenBotOptsIn pins the opt-out: a Bot with
// PreserveContext=true must NOT have its session wiped on re-activation,
// so its conversation accumulates across triggers. The follow-up still
// targets the persisted session (no churn), it just isn't cleared first.
func TestSpawnerPreservesContextWhenBotOptsIn(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	// Flip the bot to preserve context across triggers.
	bot, err := s.Bots.Get(context.Background(), "org-test", wid)
	if err != nil {
		t.Fatalf("get bot: %v", err)
	}
	if err := s.Bots.Update(context.Background(), bot.WithPreserveContext(true)); err != nil {
		t.Fatalf("update bot: %v", err)
	}
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_existing"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_existing",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if got := atomic.LoadInt32(&fc.clearCalls); got != 0 {
		t.Errorf("ClearSession called %d times; a PreserveContext bot must keep its context across triggers", got)
	}
	if fc.lastSendSID != "ses_existing" {
		t.Errorf("SendMessage sessionID = %q (want ses_existing) — follow-up must still target the persisted session", fc.lastSendSID)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Errorf("StartSession called %d times; a preserve-context follow-up must reuse the warm session", got)
	}
}

// TestSpawnerClearsSessionOnReactivationOnly drives two activations of
// the same Worker through the real Spawner closure and pins the
// context-hygiene contract end to end: the first activation opens a
// fresh session and clears nothing; the second re-uses that persisted
// session but clears it first, so each worker turn starts on a fresh
// context window instead of accumulating one ever-growing transcript.
func TestSpawnerClearsSessionOnReactivationOnly(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))

	// First activation: no persisted session → fresh StartSession, no clear.
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	if got := atomic.LoadInt32(&fc.clearCalls); got != 0 {
		t.Fatalf("first activation cleared %d times; a fresh session has no prior context to wipe", got)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Fatalf("first activation StartSession calls = %d, want 1", got)
	}

	// Second activation: the persisted session is cleared before the new
	// prompt is sent, and the same warm session is re-used (no churn).
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-2"}}); err != nil {
		t.Fatalf("spawn 2: %v", err)
	}
	if got := atomic.LoadInt32(&fc.clearCalls); got != 1 {
		t.Errorf("second activation cleared %d times, want 1", got)
	}
	fc.mu.Lock()
	clearSID, sendSID := fc.lastClearSID, fc.lastSendSID
	fc.mu.Unlock()
	if clearSID != "ses_new" {
		t.Errorf("ClearSession sessionID = %q, want ses_new (the persisted session)", clearSID)
	}
	if sendSID != "ses_new" {
		t.Errorf("SendMessage sessionID = %q, want ses_new — re-activation must re-use the warm session", sendSID)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Errorf("StartSession calls = %d after two activations; the second must re-use the session, not open a fresh one", got)
	}
}

// TestSpawnerClearFailureAbortsActivation pins fail-fast behaviour: if
// the pre-activation clear errors we must NOT silently fall through and
// dispatch the prompt onto the stale, oversized context — the whole
// point of the clear. The activation returns the error instead.
func TestSpawnerClearFailureAbortsActivation(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_existing"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_existing",
		clearErr:       errors.New("boom"),
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}})
	if err == nil {
		t.Fatal("expected activation to fail when the pre-activation clear errors")
	}
	if !strings.Contains(err.Error(), "clear session") {
		t.Errorf("error %q does not mention the clear failure", err)
	}
	if got := atomic.LoadInt32(&fc.sendCalls); got != 0 {
		t.Errorf("SendMessage called %d times after a failed clear; the prompt must NOT land on the stale context", got)
	}
}

// TestSpawnerRefusesWhenDesktopQuotaExceeded asserts the spawner fails
// fast with a useful error when Helix's `max_concurrent_desktops`
// would be exceeded by spinning up a new session. Important: only
// fires when there's no existing session — follow-ups reuse the warm
// container and must skip the check (covered by
// TestSpawnerFollowUpUsesSendSessionMessage).
func TestSpawnerRefusesWhenDesktopQuotaExceeded(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &quotaFullFakeClient{
		fakeHelixClient: fakeHelixClient{startSessionID: "ses_x"},
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	if err == nil {
		t.Fatal("expected error when quota exhausted")
	}
	if !strings.Contains(err.Error(), "quota reached") {
		t.Errorf("error %q does not mention quota", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Errorf("StartChat must NOT be called when quota is full; got %d", got)
	}
}

// quotaFullFakeClient overrides ServerStatus to report no available
// desktop slots, simulating Helix's `max_concurrent_desktops` cap.
type quotaFullFakeClient struct {
	fakeHelixClient
}

func (f *quotaFullFakeClient) ServerStatus(_ context.Context) (ServerStatus, error) {
	return ServerStatus{MaxConcurrentDesktops: 2, ActiveConcurrentDesktops: 2}, nil
}

func TestSpawnerTimeoutEmitsExitError(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []types.SessionOutputResponse{{Status: "waiting"}},
	}
	cfg := newHelixCfg(t, fc, s)
	// ActivationRunawayGuard bounds pollUntilDone. With the session
	// stuck in "waiting", the runaway guard fires and the spawner
	// returns context.DeadlineExceeded.
	cfg.ActivationRunawayGuard = 30 * time.Millisecond
	sp := Spawner(cfg)
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
}

// TestSpawnerSessionStartupTimeoutBoundsStartup pins the split between
// SessionStartupTimeout and ActivationRunawayGuard for startup work.
// A hanging StartSession must fire SessionStartupTimeout long before
// the runaway guard would. Regression test for the conflated
// ActivationTimeout that used to bound both phases.
func TestSpawnerSessionStartupTimeoutBoundsStartup(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	never := make(chan struct{}) // never closed
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		startBlock:     never,
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.SessionStartupTimeout = 30 * time.Millisecond
	cfg.ActivationRunawayGuard = 10 * time.Second // intentionally much larger

	sp := Spawner(cfg)
	start := time.Now()
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error from SessionStartupTimeout, got %v", err)
	}
	// SessionStartupTimeout was 30ms; if the bigger runaway guard had
	// bounded startup we'd see this elapsed time >>1s.
	if elapsed > 500*time.Millisecond {
		t.Fatalf("startup phase took %s — SessionStartupTimeout did not bound it (runaway guard fired instead?)", elapsed)
	}
}

// TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout pins the other
// half of the split: with SessionStartupTimeout=30ms and a fast
// startup, a session stuck in "waiting" must keep polling until the
// runaway guard fires, NOT exit at the old 30ms ActivationTimeout
// boundary. This is the regression that caused decoy interactions —
// the lane releasing on a stale startup timer while the session is
// still being polled.
func TestSpawnerPollPhaseNotBoundedBySessionStartupTimeout(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []types.SessionOutputResponse{{Status: "waiting"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.SessionStartupTimeout = 30 * time.Millisecond
	cfg.ActivationRunawayGuard = 300 * time.Millisecond

	sp := Spawner(cfg)
	start := time.Now()
	err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	elapsed := time.Since(start)
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error from runaway guard, got %v", err)
	}
	// If startup-timeout were still bounding polling we'd see ~30ms.
	// The runaway guard is 300ms; expect the deadline to land near it.
	if elapsed < 200*time.Millisecond {
		t.Fatalf("poll phase exited after %s — SessionStartupTimeout bounded the poll loop, not the runaway guard", elapsed)
	}
}

func TestSpawnerSemaphoreSerialises(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	gate := make(chan struct{})
	var inflight, peak int32
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	original := fc.outputs[0]
	fc.outputs = []types.SessionOutputResponse{original}

	cfg := newHelixCfg(t, fc, s)
	cfg.MaxInflight = 1
	cfg.ActivationRunawayGuard = time.Second

	wrapped := &concurrencyClient{inner: fc, gate: gate, inflight: &inflight, peak: &peak}
	cfg.Client = wrapped
	sp := Spawner(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(gate)
	wg.Wait()
	if got := atomic.LoadInt32(&peak); got > 1 {
		t.Errorf("peak inflight = %d (want <=1)", got)
	}
}

// concurrencyClient wraps a fakeHelixClient and tracks peak concurrent
// StartChatWithStatus calls. Used to verify the spawner's semaphore.
type concurrencyClient struct {
	inner    *fakeHelixClient
	gate     chan struct{}
	inflight *int32
	peak     *int32
}

// track records peak concurrency then blocks on the gate, so the test
// can hold multiple activations in-flight at once and assert the
// spawner's semaphore caps them.
func (c *concurrencyClient) track() func() {
	cur := atomic.AddInt32(c.inflight, 1)
	for {
		p := atomic.LoadInt32(c.peak)
		if cur <= p || atomic.CompareAndSwapInt32(c.peak, p, cur) {
			break
		}
	}
	<-c.gate
	return func() { atomic.AddInt32(c.inflight, -1) }
}

func (c *concurrencyClient) StartSession(ctx context.Context, params StartSessionParams) (string, error) {
	defer c.track()()
	return c.inner.StartSession(ctx, params)
}

func (c *concurrencyClient) SendMessage(ctx context.Context, sessionID, prompt string) error {
	defer c.track()()
	return c.inner.SendMessage(ctx, sessionID, prompt)
}

func (c *concurrencyClient) ClearSession(ctx context.Context, sessionID string) error {
	return c.inner.ClearSession(ctx, sessionID)
}

func (c *concurrencyClient) ServerStatus(ctx context.Context) (ServerStatus, error) {
	return c.inner.ServerStatus(ctx)
}

func (c *concurrencyClient) GetOutput(ctx context.Context, sid string) (types.SessionOutputResponse, error) {
	return c.inner.GetOutput(ctx, sid)
}

func (c *concurrencyClient) StopExternalAgent(ctx context.Context, sid string) error {
	return c.inner.StopExternalAgent(ctx, sid)
}

func (c *concurrencyClient) SessionOwner(ctx context.Context, sid string) (string, error) {
	return c.inner.SessionOwner(ctx, sid)
}

// An activation Ensure()s the mirror, so a turn arriving on the session
// AFTER the activation returns (e.g. inline chat) is still captured.
func TestSpawnerEnsuresSessionMirror(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	// Steady state: project + session persisted, so the activation
	// resumes ses_y and the spawner Ensures the mirror up front.
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_y"); err != nil {
		t.Fatalf("save session: %v", err)
	}

	ps := newFakePubSub()
	fc := &fakeHelixClient{
		sessionOwner:       "u-owner",
		exploratorySession: "ses_y",
		outputs:            []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.PubSub = ps
	var idCounter int32
	cfg.NewID = func() string { return fmt.Sprintf("e-%d", atomic.AddInt32(&idCounter, 1)) }
	cfg.Mirror = NewMirror(context.Background(), MirrorConfig{
		PubSub: ps, Snapshotter: NoopSessionPreamble{}, Client: fc,
		ExploratorySession: fc.ExploratorySession,
		Store:              s, Logger: cfg.Logger, NewID: cfg.NewID, Now: cfg.Now,
		PollInterval: 15 * time.Millisecond,
	})
	sp := Spawner(cfg)

	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}

	// The activation has returned, but the spawner registered the worker
	// with the mirror, which now tracks its session. A turn arriving on
	// that session from another surface (the inline chat) must still be
	// captured.
	topic := pubsub.GetSessionQueue("u-owner", "ses_y")
	if !waitForHandlers(ps, topic, 1) {
		t.Fatal("spawner did not leave a live mirror subscription for the session")
	}
	patch, _ := json.Marshal(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi there"},
	}})
	ps.publish(t, topic, patch)
	complete, _ := json.Marshal(types.WebsocketEvent{Interaction: &types.Interaction{State: "complete"}})
	ps.publish(t, topic, complete)

	if !waitForSegment(t, s, wid, "assistant: hi there") {
		t.Fatal("post-activation session turn not mirrored to the transcript")
	}
}

// A follow-up must never churn to a fresh session: SendMessage is
// fire-and-forget and Helix auto-resumes a downed desktop on the same
// session, so the persisted pointer stays intact.
func TestSpawnerFollowUpSurvivesDownDesktop(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_existing"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &fakeHelixClient{
		startSessionID: "ses_should_not_be_used",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Errorf("StartSession called %d times; a follow-up must never create a fresh session (no churn)", got)
	}
	state, _ := LoadState(context.Background(), s, "org-test", wid)
	if state.SessionID != "ses_existing" {
		t.Errorf("session pointer = %q, want ses_existing (follow-up must keep the same session)", state.SessionID)
	}
}

// TestSpawnerRecordsActivationRowOnSuccess pins B5.6 — the Spawner
// MUST create an activation row at start and complete it with
// StatusOK at end, so the audit/replay surface stays in sync with
// the transcript topic. The id derives from cfg.NewID with the
// "a-" prefix; StartedAt/EndedAt come from cfg.Now; TranscriptID
// is the canonical TopicID derivation; Outcome.Status reflects the
// Spawner's return value.
func TestSpawnerRecordsActivationRowOnSuccess(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	rows, err := s.Activations.ListForWorker(context.Background(), "org-test", wid, 10)
	if err != nil {
		t.Fatalf("list activations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 activation per spawn", len(rows))
	}
	a := rows[0]
	if a.WorkerID != wid {
		t.Errorf("row.WorkerID = %q, want %q", a.WorkerID, wid)
	}
	if a.ID != activation.ID("a-id") {
		t.Errorf("row.ID = %q, want a-id (from NewID stub)", a.ID)
	}
	if a.TranscriptID != activation.TranscriptID(wid) {
		t.Errorf("row.TranscriptID = %q, want %q", a.TranscriptID, activation.TranscriptID(wid))
	}
	if !a.IsCompleted() {
		t.Fatalf("row not marked completed; EndedAt = %v", a.EndedAt)
	}
	if a.Outcome.Status != activation.StatusOK {
		t.Errorf("Outcome.Status = %q, want ok", a.Outcome.Status)
	}
	if a.Outcome.Error != "" {
		t.Errorf("Outcome.Error = %q, want empty on success", a.Outcome.Error)
	}
}

// TestSpawnerRecordsActivationRowOnError pins the failure path: a
// Spawner error still records an activation row with StatusError
// and the wrapped err.Error() text.
func TestSpawnerRecordsActivationRowOnError(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startErr: errors.New("desktop quota exceeded"),
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.SessionStartupTimeout = time.Second
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err == nil {
		t.Fatal("spawn: nil error, want quota error")
	}
	rows, err := s.Activations.ListForWorker(context.Background(), "org-test", wid, 10)
	if err != nil {
		t.Fatalf("list activations: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("len(rows) = %d, want 1 (failed activations still recorded)", len(rows))
	}
	a := rows[0]
	if !a.IsCompleted() {
		t.Fatal("row not completed on Spawner error")
	}
	if a.Outcome.Status != activation.StatusError {
		t.Errorf("Outcome.Status = %q, want error", a.Outcome.Status)
	}
	if a.Outcome.Error == "" {
		t.Error("Outcome.Error empty on error path")
	}
}

// TestSpawnerHonorsSharedSemaphore pins the mechanism that replaced the
// cross-tenant-leaking cached spawner
// (design/2026-06-09-org-multitenancy-spawner-leak.md).
//
// The old host wrapper cached ONE inner Spawner (and so one org's
// frozen OrgID/HelixOrgURL) to share a single inflight cap. The fix
// rebuilds a per-org SpawnerConfig every activation and instead shares
// the cap via SpawnerConfig.Sem. If Spawner ignores cfg.Sem and mints
// its own semaphore from MaxInflight, the global cap is silently lost.
//
// Here we inject a shared semaphore whose only slot is already taken.
// A correct Spawner blocks on it and returns the context error without
// starting a session; a Spawner that ignored cfg.Sem would sail past
// and call StartChat.
func TestSpawnerHonorsSharedSemaphore(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	sem := make(chan struct{}, 1)
	sem <- struct{}{} // occupy the only slot — no inflight budget left
	cfg.Sem = sem

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := Spawner(cfg)(ctx, "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded while the shared semaphore is full, got %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 0 {
		t.Fatalf("StartChat fired %d times despite a full shared semaphore — Spawner ignored cfg.Sem and the global inflight cap is lost", got)
	}

	// Free the slot; the next activation must now proceed to completion,
	// proving the gate was the semaphore and nothing else.
	<-sem
	if err := Spawner(cfg)(context.Background(), "org-test", wid, []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("activation with a free shared-semaphore slot must succeed, got %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Fatalf("StartChat calls = %d, want 1 after the slot freed", got)
	}
}
