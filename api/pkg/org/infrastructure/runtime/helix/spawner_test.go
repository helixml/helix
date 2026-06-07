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
	mu             sync.Mutex
	startCalls     int32
	sendCalls      int32
	outputCalls    int32
	subscribeCalls int32
	startSessionID string
	startErr       error
	sendErr        error
	outputs        []types.SessionOutputResponse
	updatesFactory func() <-chan types.WebsocketEvent
	lastStartReq   StartChatRequest
	lastSendSID    string
	lastSendBody   string
}

func (f *fakeHelixClient) StartChatWithStatus(_ context.Context, req StartChatRequest) (types.Session, bool, error) {
	atomic.AddInt32(&f.startCalls, 1)
	f.mu.Lock()
	f.lastStartReq = req
	f.mu.Unlock()
	return types.Session{ID: f.startSessionID}, false, f.startErr
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
func (f *fakeHelixClient) ServerStatus(_ context.Context) (ServerStatus, error) {
	return ServerStatus{MaxConcurrentDesktops: 0, ActiveConcurrentDesktops: 0}, nil
}

func newHelixTestStore(t *testing.T) (*store.Store, orgchart.WorkerID) {
	t.Helper()
	s := orggorm.GetOrgTestDB(t)
	ctx := context.Background()
	role, _ := orgchart.NewRole("r-eng", "# Role: Engineer", nil, nil, time.Now().UTC(), "org-test")
	if err := s.Roles.Create(ctx, role); err != nil {
		t.Fatalf("role: %v", err)
	}
	worker, _ := orgchart.NewAIWorker("w-eng", "r-eng", "# Persona", "org-test")
	if err := s.Workers.Create(ctx, worker); err != nil {
		t.Fatalf("worker: %v", err)
	}
	return s, worker.ID()
}

func newHelixCfg(t *testing.T, fc SpawnerClient, s *store.Store) SpawnerConfig {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return SpawnerConfig{
		Client:            fc,
		ProjectService:    newFakeProjectService(),
		Workspace:         NewWorkspace(newFakeGitForProject(), s, "helix-specs", "helix-org", "ho@example.com"),
		PubSub:            newFakePubSub(),
		Snapshotter:       NoopSessionPreamble{},
		HelixOrgURL:       "http://helix-org:8081",
		Provider:          "openai",
		Model:             "gpt-4o-mini",
		ActivationTimeout: 2 * time.Second,
		MaxInflight:       2,
		PollInitial:       time.Millisecond,
		PollMax:           5 * time.Millisecond,
		Logger:            logger,
		Store:             s,
		Now:               func() time.Time { return time.Now().UTC() },
		NewID:             func() string { return "id" },
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
	err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if got := atomic.LoadInt32(&fc.startCalls); got != 1 {
		t.Errorf("StartChat calls: %d", got)
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
	// StartChat must point at the per-Worker project, not at any
	// global one.
	if fc.lastStartReq.ProjectID != "prj_test" {
		t.Errorf("StartChat ProjectID = %q (want prj_test)", fc.lastStartReq.ProjectID)
	}
	if fc.lastStartReq.AppID != "app_test" {
		t.Errorf("StartChat AppID = %q (want app_test)", fc.lastStartReq.AppID)
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
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn 1: %v", err)
	}
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerEvent}}); err != nil {
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
// EntryStream callback produces the same line shapes the claude
// bridge emits — assistant text, tool_use, tool_result.
func TestBridgeRendersEntryPatchEvents(t *testing.T) {
	t.Parallel()
	var got []string
	b := newBridge(func(s string) { got = append(got, s) })
	b.stream.Apply(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi", PatchOffset: 0},
	}})
	b.stream.Apply(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 1, MessageID: "t1", Type: "tool_call", Patch: `{"x":1}`, ToolName: "publish", ToolStatus: "Completed"},
	}})
	b.stream.Flush()
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
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e-1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// The first StartChatWithStatus call (resume) carries the existing
	// SessionID. The session pointer in the store must remain unchanged.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastStartReq.SessionID != "ses_existing" {
		t.Errorf("StartChatRequest.SessionID = %q (want ses_existing) — resume must target persisted session", fc.lastStartReq.SessionID)
	}
	state, _ := LoadState(context.Background(), s, "org-test", wid)
	if state.SessionID != "ses_existing" {
		t.Errorf("session pointer changed to %q; resume must NOT open a fresh session", state.SessionID)
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
	err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
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

// TestSpawnerColdStartReQueues verifies that when StartChatWithStatus
// reports hadStreamErr=true on the fresh open, EnsureAndSend re-issues
// the same prompt against the same session ID (belt-and-braces — the
// original race that made this critical was fixed in Helix; this
// retry is the fallback).
func TestSpawnerColdStartReQueues(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &coldStartFakeClient{
		fakeHelixClient: fakeHelixClient{
			startSessionID: "ses_new",
			outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
		},
		hadWSError: true,
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	// Two StartChatWithStatus calls: the fresh open and the retry on
	// the same session. (Cold-start retry replaced the older
	// SendSessionMessage path in EnsureAndSend.)
	if got := atomic.LoadInt32(&fc.startCalls); got < 2 {
		t.Errorf("StartChat calls: %d (want >=2 — fresh open + cold-start retry)", got)
	}
	// Retry targets the freshly-opened session.
	fc.mu.Lock()
	defer fc.mu.Unlock()
	if fc.lastStartReq.SessionID != "ses_new" {
		t.Errorf("retry SessionID = %q (want ses_new — retry on same session)", fc.lastStartReq.SessionID)
	}
}

// coldStartFakeClient overrides StartChatWithStatus to return
// hadWSError=true, simulating Helix's "no agent WS yet" race.
type coldStartFakeClient struct {
	fakeHelixClient
	hadWSError bool
}

func (f *coldStartFakeClient) StartChatWithStatus(_ context.Context, req StartChatRequest) (types.Session, bool, error) {
	atomic.AddInt32(&f.startCalls, 1)
	f.mu.Lock()
	f.lastStartReq = req
	f.mu.Unlock()
	return types.Session{ID: f.startSessionID}, f.hadWSError, f.startErr
}

func TestSpawnerTimeoutEmitsExitError(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_x",
		outputs:        []types.SessionOutputResponse{{Status: "waiting"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.ActivationTimeout = 30 * time.Millisecond
	sp := Spawner(cfg)
	err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
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
	cfg.ActivationTimeout = time.Second

	wrapped := &concurrencyClient{inner: fc, gate: gate, inflight: &inflight, peak: &peak}
	cfg.Client = wrapped
	sp := Spawner(cfg)

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
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

func (c *concurrencyClient) StartChatWithStatus(ctx context.Context, req StartChatRequest) (types.Session, bool, error) {
	cur := atomic.AddInt32(c.inflight, 1)
	for {
		p := atomic.LoadInt32(c.peak)
		if cur <= p || atomic.CompareAndSwapInt32(c.peak, p, cur) {
			break
		}
	}
	defer atomic.AddInt32(c.inflight, -1)
	<-c.gate
	return c.inner.StartChatWithStatus(ctx, req)
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

// TestSpawnerPublishesTranscriptViaEntryStream verifies the bridge
// subscribes via pubsub.GetSessionQueue, feeds frames through
// EntryStream, and republishes settled events as activation Stream
// events.
func TestSpawnerPublishesTranscriptViaEntryStream(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)

	ps := newFakePubSub()
	fc := &fakeHelixClient{
		startSessionID: "ses_y",
		// Several waiting outputs so the bridge has time to consume
		// the pubsub frames before pollUntilDone terminates.
		outputs: []types.SessionOutputResponse{
			{Status: "waiting"}, {Status: "waiting"}, {Status: "complete", Output: "ok"},
		},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.PubSub = ps
	cfg.PollInitial = 80 * time.Millisecond
	cfg.ActivationTimeout = 2 * time.Second
	// Unique IDs so each publishActivationEvent insert succeeds.
	var idCounter int32
	cfg.NewID = func() string { return fmt.Sprintf("e-%d", atomic.AddInt32(&idCounter, 1)) }
	sp := Spawner(cfg)

	// Drive the activation in a goroutine so we can publish frames
	// after the bridge has subscribed.
	done := make(chan error, 1)
	go func() {
		done <- sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}})
	}()

	// Wait for the bridge to subscribe (handlers map populated).
	deadline := time.Now().Add(2 * time.Second)
	topic := pubsub.GetSessionQueue("", "ses_y")
	for time.Now().Before(deadline) {
		if ps.handlerCount(topic) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	patch, _ := json.Marshal(types.WebsocketEvent{EntryPatches: []types.EntryPatch{
		{Index: 0, MessageID: "m1", Type: "text", Patch: "hi there"},
	}})
	ps.publish(t, topic, patch)
	complete, _ := json.Marshal(types.WebsocketEvent{Interaction: &types.Interaction{State: "complete"}})
	ps.publish(t, topic, complete)

	if err := <-done; err != nil {
		t.Fatalf("spawn: %v", err)
	}

	events, err := s.Events.ListForStream(context.Background(), "org-test", activation.StreamID(wid), 100)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	var sawAssistant bool
	for _, e := range events {
		msg, err := e.Message()
		if err != nil {
			continue
		}
		if strings.Contains(msg.Body, "assistant: hi there") {
			sawAssistant = true
		}
	}
	if !sawAssistant {
		t.Fatalf("activation stream missing transcript line; events: %+v", events)
	}
}

// TestSpawnerOpensFreshOnStaleSession: when the persisted session ID
// resume fails (Helix reports hadStreamErr), the spawner opens a
// fresh session and persists the new ID.
func TestSpawnerOpensFreshOnStaleSession(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	if err := SaveProject(context.Background(), s, "org-test", wid, "prj_test", "app_test", "repo_test"); err != nil {
		t.Fatalf("save project: %v", err)
	}
	if err := SaveSession(context.Background(), s, "org-test", wid, "ses_stale"); err != nil {
		t.Fatalf("save session: %v", err)
	}
	fc := &staleSessionFake{
		fakeHelixClient: fakeHelixClient{
			startSessionID: "ses_fresh",
			outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
		},
	}
	cfg := newHelixCfg(t, &fc.fakeHelixClient, s)
	cfg.Client = fc
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerEvent, EventID: "e1"}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	state, _ := LoadState(context.Background(), s, "org-test", wid)
	if state.SessionID != "ses_fresh" {
		t.Errorf("session pointer = %q, want ses_fresh (stale resume must fall through to fresh)", state.SessionID)
	}
}

// staleSessionFake reports hadStreamErr=true when a resume call is
// made (SessionID != "") — simulating Helix's "session no longer
// running" signal. Fresh opens (SessionID empty) succeed normally.
type staleSessionFake struct {
	fakeHelixClient
}

func (f *staleSessionFake) StartChatWithStatus(_ context.Context, req StartChatRequest) (types.Session, bool, error) {
	atomic.AddInt32(&f.startCalls, 1)
	f.mu.Lock()
	f.lastStartReq = req
	f.mu.Unlock()
	hadErr := req.SessionID != "" // resume path → "session no longer running"
	return types.Session{ID: f.startSessionID}, hadErr, f.startErr
}

// TestSpawnerRecordsActivationRowOnSuccess pins B5.6 — the Spawner
// MUST create an activation row at start and complete it with
// StatusOK at end, so the audit/replay surface stays in sync with
// the transcript stream. The id derives from cfg.NewID with the
// "a-" prefix; StartedAt/EndedAt come from cfg.Now; TranscriptStreamID
// is the canonical StreamID derivation; Outcome.Status reflects the
// Spawner's return value.
func TestSpawnerRecordsActivationRowOnSuccess(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	sp := Spawner(newHelixCfg(t, fc, s))
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
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
	if a.TranscriptStreamID != activation.StreamID(wid) {
		t.Errorf("row.TranscriptStreamID = %q, want %q", a.TranscriptStreamID, activation.StreamID(wid))
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

// TestSpawnerRunsRegisteredSecretInjectors pins the
// transport→spawner secret-injection plumbing. When the spawner is
// configured with one or more SpawnSecretInjector instances, every
// activation must call each one and upsert the returned secrets as
// project secrets so the desktop container's runtime can surface
// them as env vars.
//
// Pin via the github-shaped case (GH_TOKEN), but the spawner has
// no GitHub awareness — the injector is just a generic
// SpawnSecretInjectorFunc, exactly like Postmark or any future
// transport would register.
func TestSpawnerRunsRegisteredSecretInjectors(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.SecretInjectors = []SpawnSecretInjector{
		SpawnSecretInjectorFunc{
			Label: "github",
			Fn: func(_ context.Context, orgID string) (map[string]string, error) {
				if orgID != "org-test" {
					t.Errorf("injector got orgID = %q, want org-test", orgID)
				}
				return map[string]string{"GH_TOKEN": "gho_token_abc"}, nil
			},
		},
	}
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	svc := cfg.ProjectService.(*fakeProjectService)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.putSecretLast["GH_TOKEN"] != "gho_token_abc" {
		t.Errorf("GH_TOKEN secret = %q, want %q (set keys: %v)", svc.putSecretLast["GH_TOKEN"], "gho_token_abc", svc.putSecretLast)
	}
}

// TestSpawnerSkipsInjectorReturningEmptyMap pins the degraded-mode
// path: an injector returning an empty map (e.g. "operator hasn't
// connected this transport's auth yet") must NOT cause the
// spawner to upsert an empty secret. That would shadow any
// pre-existing value in the sandbox container.
func TestSpawnerSkipsInjectorReturningEmptyMap(t *testing.T) {
	t.Parallel()
	s, wid := newHelixTestStore(t)
	fc := &fakeHelixClient{
		startSessionID: "ses_new",
		outputs:        []types.SessionOutputResponse{{Status: "complete", Output: "ok"}},
	}
	cfg := newHelixCfg(t, fc, s)
	cfg.SecretInjectors = []SpawnSecretInjector{
		SpawnSecretInjectorFunc{
			Label: "github",
			Fn: func(_ context.Context, _ string) (map[string]string, error) {
				// Mirror the github injector's "no OAuth wired yet"
				// behaviour: nil map, no error.
				return nil, nil
			},
		},
	}
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err != nil {
		t.Fatalf("spawn: %v", err)
	}
	svc := cfg.ProjectService.(*fakeProjectService)
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, set := svc.putSecretLast["GH_TOKEN"]; set {
		t.Errorf("GH_TOKEN should NOT be set when injector returns empty; got %q", svc.putSecretLast["GH_TOKEN"])
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
	cfg.ActivationTimeout = time.Second
	sp := Spawner(cfg)
	if err := sp(context.Background(), "org-test", wid, "/ignored", []activation.Trigger{{Kind: activation.TriggerHire}}); err == nil {
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
